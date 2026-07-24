package lifecycle

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/solution"
)

//go:embed assets/clm-form-definition.json
var bundledCLMFormDefinition []byte

type boxPrivateRequest struct {
	Operation string                    `json:"operation"`
	Form      *boxPrivateFormRequest    `json:"form,omitempty"`
	App       *boxPrivateAppRequest     `json:"app,omitempty"`
	Resources map[string]privateBoxLink `json:"resources,omitempty"`
}

type boxPrivateFormRequest struct {
	Title      string          `json:"title"`
	Definition json.RawMessage `json:"definition,omitempty"`
	FolderID   string          `json:"folderId,omitempty"`
}

type boxPrivateAppRequest struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	TemplateTitle string `json:"templateTitle"`
	FormTitle     string `json:"formTitle,omitempty"`
}

type privateBoxLink struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	URL  string `json:"url,omitempty"`
}

type boxPrivateResponse struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Result struct {
		Form *struct {
			Present       bool   `json:"present"`
			Outcome       string `json:"outcome"`
			Title         string `json:"title"`
			ID            string `json:"id"`
			FormID        string `json:"formId"`
			FileRequestID string `json:"fileRequestId"`
			URLID         string `json:"urlId"`
		} `json:"form,omitempty"`
		App *struct {
			Present      bool   `json:"present"`
			Outcome      string `json:"outcome"`
			Title        string `json:"title"`
			ID           string `json:"id"`
			PageCount    int    `json:"pageCount"`
			SectionCount int    `json:"sectionCount"`
			BlockCount   int    `json:"blockCount"`
		} `json:"app,omitempty"`
	} `json:"result"`
}

// validateBoxPrivateAdapters records the Box private surfaces (Forms and Apps)
// as deployable without inspecting them. Those surfaces have no read API, so a
// status check would have to drive an authenticated browser tab. Keeping
// validate read-only and non-interactive, the surfaces are marked for automatic
// creation and the deploy step provisions them through the browser.
func validateBoxPrivateAdapters(root string, manifest solution.Manifest, settings solution.BoxDeploymentSettings, selection solution.ComponentSelection, item *Item) error {
	_, components, err := privateAdapterRequest(root, manifest, settings, selection, "inspect", nil, "", nil)
	if err != nil || len(components) == 0 {
		return err
	}
	for _, key := range []string{"form", "app"} {
		if component, ok := components[key]; ok {
			classifyBoxComponent(item, component, false, true)
		}
	}
	return nil
}

func deployBoxPrivateAdapters(root string, manifest solution.Manifest, settings solution.BoxDeploymentSettings, selection solution.ComponentSelection, deployable []string, workspaceID string, existing []ResourceReference) ([]string, []ResourceReference, error) {
	resourceLinks := privateResourceLinks(existing)
	resourceLinks[manifest.Box.Workspace.Name] = privateBoxLink{ID: workspaceID, Kind: "folder"}
	if approved, found := resourceLinks["Approved Clauses"]; found {
		resourceLinks["09 - Clause Library"] = approved
	}
	request, components, err := privateAdapterRequest(root, manifest, settings, selection, "deploy", resourceLinks, workspaceID, deployable)
	if err != nil {
		return nil, nil, err
	}
	if len(components) == 0 {
		return nil, nil, nil
	}
	response, err := executeBoxPrivateBrowser(request)
	if err != nil {
		return nil, nil, fmt.Errorf("deploy Box Forms and Apps through the authenticated browser: %w", err)
	}
	deployed := []string{}
	resources := []ResourceReference{}
	if request.Form != nil && response.Result.Form != nil {
		component := components["form"]
		deployed = append(deployed, component)
		id := firstNonEmpty(response.Result.Form.FileRequestID, response.Result.Form.FormID, response.Result.Form.ID)
		resources = append(resources, ResourceReference{Provider: "box", Component: component, Kind: "form", Name: response.Result.Form.Title, ID: id})
	}
	if request.App != nil && response.Result.App != nil {
		component := components["app"]
		deployed = append(deployed, component)
		resources = append(resources, ResourceReference{Provider: "box", Component: component, Kind: "app", Name: response.Result.App.Title, ID: response.Result.App.ID})
	}
	return deployed, resources, nil
}

func privateAdapterRequest(root string, manifest solution.Manifest, settings solution.BoxDeploymentSettings, selection solution.ComponentSelection, operation string, resources map[string]privateBoxLink, workspaceID string, deployable []string) (boxPrivateRequest, map[string]string, error) {
	request := boxPrivateRequest{Operation: operation, Resources: resources}
	components := map[string]string{}
	formCapability, formEnabled := enabledCapability(manifest, selection, "Box Form")
	formTitle := ""
	if formEnabled && formCapability.Handler == "box.private-form" {
		var err error
		formTitle, err = solution.ResolveDeploymentName(formCapability.DisplayName, settings)
		if err != nil {
			return request, nil, err
		}
		component := "Box Form:" + formCapability.DisplayName
		components["form"] = component
		if operation == "inspect" || operation == "destroy" || slices.Contains(deployable, component) {
			form := &boxPrivateFormRequest{Title: formTitle, FolderID: privateFolderID(resources, "01 - Intake", workspaceID)}
			if operation == "deploy" {
				data, err := os.ReadFile(filepath.Join(root, "config", "box", filepath.FromSlash(formCapability.Source)))
				if errors.Is(err, os.ErrNotExist) {
					data = bundledCLMFormDefinition
					err = nil
				}
				if err != nil {
					return request, nil, fmt.Errorf("read Box Form definition: %w", err)
				}
				if !json.Valid(data) {
					return request, nil, fmt.Errorf("Box Form definition is not valid JSON")
				}
				form.Definition = data
			}
			request.Form = form
		}
	}
	appCapability, appEnabled := enabledCapability(manifest, selection, "Box App")
	if appEnabled && appCapability.Handler == "box.private-app" {
		appTitle, err := solution.ResolveDeploymentName(appCapability.DisplayName, settings)
		if err != nil {
			return request, nil, err
		}
		component := "Box App:" + appCapability.DisplayName
		components["app"] = component
		if operation == "inspect" || operation == "destroy" || slices.Contains(deployable, component) {
			request.App = &boxPrivateAppRequest{
				Title: appTitle, Description: "Operational CLM cockpit for governed intake, document risk, approvals, approved clauses, execution, and renewal readiness.",
				TemplateTitle: firstNonEmpty(appCapability.Template, appCapability.DisplayName), FormTitle: formTitle,
			}
		}
	}
	if request.Form == nil {
		delete(components, "form")
	}
	if request.App == nil {
		delete(components, "app")
	}
	return request, components, nil
}

// destroyBoxPrivateSurfaces removes the Box Form and Box App through the
// authenticated browser, the only mechanism that can delete them. It returns an
// outcome per recorded private resource plus the set it handled, so the API
// teardown pass can skip them. Browser failures are reported per resource rather
// than aborting the reset.
func destroyBoxPrivateSurfaces(root string, box boxContext, resources []ResourceReference) ([]TeardownOutcome, map[string]bool) {
	handled := map[string]bool{}
	outcomes := []TeardownOutcome{}
	pending := []ResourceReference{}
	for _, resource := range resources {
		if resource.Kind == "form" || resource.Kind == "app" {
			pending = append(pending, resource)
		}
	}
	if len(pending) == 0 {
		return outcomes, handled
	}

	fail := func(reason string) ([]TeardownOutcome, map[string]bool) {
		for _, resource := range pending {
			outcomes = append(outcomes, TeardownOutcome{Resource: resource, Error: reason})
			handled[resourceKey(resource)] = true
		}
		return outcomes, handled
	}

	request, components, err := privateAdapterRequest(root, box.manifest, box.settings.Box, box.selection, "destroy", nil, "", nil)
	if err != nil {
		return fail("build private surface request: " + err.Error())
	}
	if len(components) == 0 {
		return fail("no private surface adapter is configured for this solution")
	}
	response, err := executeBoxPrivateBrowser(request)
	if err != nil {
		return fail("remove through the authenticated browser: " + err.Error())
	}

	for _, resource := range pending {
		outcome := TeardownOutcome{Resource: resource}
		switch resource.Kind {
		case "form":
			if response.Result.Form == nil {
				outcome.Error = "the browser returned no Box Form result"
			} else {
				applyPrivateDestroyOutcome(&outcome, response.Result.Form.Outcome, "Box Form")
			}
		case "app":
			if response.Result.App == nil {
				outcome.Error = "the browser returned no Box App result"
			} else {
				applyPrivateDestroyOutcome(&outcome, response.Result.App.Outcome, "Box App")
			}
		}
		outcomes = append(outcomes, outcome)
		handled[resourceKey(resource)] = true
	}
	return outcomes, handled
}

// applyPrivateDestroyOutcome records a delete only when the browser reported it
// actually happened. "absent" must never count as deleted: an unauthenticated
// session makes every surface look absent (the Box app tier answers 200 with an
// empty list rather than 401), which would otherwise report a reset that never
// removed anything.
func applyPrivateDestroyOutcome(outcome *TeardownOutcome, reported, label string) {
	switch reported {
	case "deleted":
		outcome.Deleted = true
	case "absent":
		outcome.Error = fmt.Sprintf("%s was not found in Box; nothing was removed", label)
	default:
		outcome.Error = firstNonEmpty(reported, label+" was not removed")
	}
}

func privateResourceLinks(resources []ResourceReference) map[string]privateBoxLink {
	links := map[string]privateBoxLink{}
	for _, resource := range resources {
		if resource.Provider == "box" && resource.Name != "" && resource.ID != "" {
			links[resource.Name] = privateBoxLink{ID: resource.ID, Kind: resource.Kind, URL: resource.URL}
		}
	}
	return links
}

func privateFolderID(resources map[string]privateBoxLink, name, fallback string) string {
	if resource, found := resources[name]; found && resource.ID != "" {
		return resource.ID
	}
	return fallback
}

func writeBoxPrivateCapture(result []byte) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	directory := filepath.Join(home, "Library", "Application Support", "box-dispatch", "network-captures")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	name := "private-api-" + time.Now().UTC().Format("20060102T150405Z") + ".json"
	var formatted bytes.Buffer
	if json.Indent(&formatted, result, "", "  ") != nil {
		formatted.Write(result)
	}
	return os.WriteFile(filepath.Join(directory, name), formatted.Bytes(), 0o600)
}

func boxPrivateBrowserScript(payload string) string {
	return fmt.Sprintf(`window.__boxDispatchPrivateResult=null;window.__boxDispatchPrivateNetwork=[];(async()=>{
"use strict";
const request=%s;
if(!location.hostname.endsWith(".ent.box.com"))throw new Error("not signed in to Box: expected a <tenant>.ent.box.com tab but this tab is on "+location.hostname+". Sign in to Box in the box-dispatch browser window, then retry");
const network=window.__boxDispatchPrivateNetwork;
const capturedFetch=async(url,options={})=>{const entry={startedAt:new Date().toISOString(),method:options.method||"GET",url:String(url),requestBody:null,status:null,responseBody:null};if(options.body instanceof FormData){entry.requestBody={};for(const [key,value] of options.body.entries())entry.requestBody[key]=value}else if(typeof options.body==="string"){try{entry.requestBody=JSON.parse(options.body)}catch{entry.requestBody=options.body}}network.push(entry);const controller=new AbortController();const timeout=setTimeout(()=>controller.abort(),15000);try{const response=await fetch(url,{...options,signal:controller.signal});entry.status=response.status;const responseText=await response.clone().text();entry.responseBody=responseText;try{entry.responseBody=JSON.parse(responseText)}catch{}return response}finally{clearTimeout(timeout)}};
const sortValue=value=>Array.isArray(value)?value.map(sortValue):(value&&typeof value==="object")?Object.fromEntries(Object.keys(value).sort().map(key=>[key,sortValue(value[key])])):value;
const canonical=v=>JSON.stringify(sortValue(v));
const meteorBase="/app-api/crooze/call-meteor-method/v1/";
const meteor=async(method,args)=>{const response=await capturedFetch(meteorBase+method,{method:"POST",credentials:"include",headers:{"content-type":"application/json"},body:JSON.stringify(args)});const body=await response.json();if(!response.ok)throw new Error(method+" failed: "+(body?.message||"HTTP "+response.status));return body};
const forms=async(path,options={})=>{const response=await capturedFetch("/app-api/file-request-web"+path,{credentials:"include",...options});const body=response.status===204?null:await response.json();if(!response.ok)throw new Error("Forms API failed: "+(body?.message||body?.error||"HTTP "+response.status));return body};
const formEntries=body=>Array.isArray(body)?body:(body?.data||body?.entries||body?.items||body?.fileRequests||[]);
const multipart=values=>{const data=new FormData();for(const [key,value] of Object.entries(values))data.append(key,typeof value==="string"?value:JSON.stringify(value));return data};
const stableId=async key=>{const bytes=await crypto.subtle.digest("SHA-256",new TextEncoder().encode(key));return "element-"+btoa(String.fromCharCode(...new Uint8Array(bytes).slice(0,16))).replaceAll("+","-").replaceAll("/","_").replaceAll("=","")};
const formContent=async(spec,folderId)=>{const items=[],layout=[],components={"group-0":{id:"group-0",type:"group",label:spec.name,description:String(spec.description||""),items}};let y=0;for(const field of spec.fields){const id=await stableId(field.key);const common={required:field.required,label:field.label,id};let component;if(field.type==="shortText")component={type:"textField",textType:"text",...common};else if(field.type==="longText")component={type:"textField",textType:"text",multiline:true,...common};else if(field.type==="email")component={type:"textField",textType:"email",visible:true,...common};else if(field.type==="number")component={type:"numberField",...common};else if(field.type==="dropdown")component={type:"selectField",maximumSelections:0,options:field.options,...common};else if(field.type==="date")component={type:"dateTimeField",dateTimeMode:"date",dateLabel:"",timeLabel:"",...common};else if(field.type==="fileUpload"){if(!folderId)throw new Error("Box Form file upload requires a resolved intake folder ID");component={type:"uploadField",folderId,showFileDescription:false,...common}}else throw new Error("Unsupported Box Form field type: "+field.type);const height={date:151,fileUpload:391,longText:188}[field.type]||148;items.push(id);layout.push({w:2,h:height,x:0,y,i:id,moved:false,static:false});y+=height;components[id]=component}return {root:"group-0",layouts:{"group-0":{layout}},components,theme:null,type:"form"}};
const findForms=async title=>{const listed=await forms("/file-requests?limit=20&sortDirection=DESC&sortField=modifiedAt&type=form");return formEntries(listed).filter(item=>(item?.title||item?.form?.title)===title)};
const resolveForm=async title=>{const matches=await findForms(title);if(matches.length>1)throw new Error("Duplicate Box Form title: "+title);if(!matches.length)return null;const summary=matches[0],id=summary?.fileRequestId||summary?.id||summary?.formId;if(!id)throw new Error("Box Form match has no identifier");const detail=await forms("/file-request/"+id);return {summary,detail,id}};
const destroyForm=async(current,title)=>{if(!current)return {present:false,outcome:"absent",title,id:""};const id=String(current.id);try{await forms("/file-request/"+id,{method:"DELETE"});return {present:false,outcome:"deleted",title,id}}catch(error){return {present:true,outcome:"Box Form delete failed: "+String(error?.message||error),title,id}}};
const destroyApp=async(summary,title)=>{if(!summary)return {present:false,outcome:"absent",title,id:""};const id=String(summary._id);const attempts=["app.remove","app.delete","app.archive"];let lastError=null;for(const method of attempts){try{await meteor(method,[id]);return {present:false,outcome:"deleted",title,id}}catch(error){lastError=error}}return {present:true,outcome:"Box App delete failed: "+String(lastError?.message||lastError),title,id}};
const result={};
if(request.form){let current=await resolveForm(request.form.title);if(request.operation==="destroy")result.form=await destroyForm(current,request.form.title);else if(request.operation==="inspect")result.form={present:!!current,title:request.form.title,id:current?.id||"",formId:String(current?.detail?.form?.id||current?.summary?.formId||""),fileRequestId:String(current?.id||""),urlId:String(current?.detail?.urlId||current?.detail?.form?.urlId||current?.summary?.urlId||"")};else{const spec={...request.form.definition,name:request.form.title};const desired=await formContent(spec,request.form.folderId);let outcome="unchanged";if(!current){await forms("/form",{method:"POST",body:multipart({title:request.form.title,content:desired})});outcome="created";current=await resolveForm(request.form.title);if(!current)throw new Error("Box Form create verification failed")}else{const raw=current.detail?.form?.content;const existing=typeof raw==="string"?JSON.parse(raw):raw;if(canonical(existing)!==canonical(desired)){const versionId=current.detail?.form?.versionId||current.detail?.formVersion?.id||current.summary?.formVersionId;if(!versionId)throw new Error("Existing Box Form has no version identifier");await forms("/form-version/"+versionId,{method:"POST",body:multipart({fileRequestId:current.id,content:desired})});outcome="updated";current=await resolveForm(request.form.title)}}result.form={present:true,outcome,title:request.form.title,id:String(current.id||""),formId:String(current.detail?.form?.id||current.summary?.formId||""),fileRequestId:String(current.id||""),urlId:String(current.detail?.urlId||current.detail?.form?.urlId||current.summary?.urlId||"")}}}
if(request.app){const listed=await meteor("app.list",[]);const exact=(listed.apps||[]).filter(app=>app?.name===request.app.title);if(exact.length>1)throw new Error("Duplicate Box App title: "+request.app.title);if(request.operation==="destroy")result.app=await destroyApp(exact[0],request.app.title);else if(request.operation==="inspect")result.app={present:exact.length===1,title:request.app.title,id:exact[0]?._id||""};else{const templates=(listed.apps||[]).filter(app=>app?.name===request.app.templateTitle);if(!templates.length)throw new Error("Box App template not found: "+request.app.templateTitle);let targetSummary=exact[0],outcome="updated";if(!targetSummary){await meteor("app.create",[{name:request.app.title,initialPageName:"Home"}]);const refreshed=await meteor("app.list",[]);targetSummary=(refreshed.apps||[]).find(app=>app?.name===request.app.title);outcome="created"}if(!targetSummary)throw new Error("Box App create verification failed");const source=await meteor("app.get",[templates[0]._id]);const target=await meteor("app.get",[targetSummary._id]);const randomId=()=>crypto.randomUUID().replaceAll("-","").slice(0,17);const rewriteEnterprise=value=>typeof value==="string"?value.replaceAll(/enterprise_\d+/g,"enterprise_"+target.enterpriseId):Array.isArray(value)?value.map(rewriteEnterprise):(value&&typeof value==="object")?Object.fromEntries(Object.entries(value).map(([k,v])=>[k,rewriteEnterprise(v)])):value;const formRef=request.app.formTitle?await resolveForm(request.app.formTitle):null;const pages=source.pages.map((sourcePage,pageIndex)=>{const pageId=pageIndex===0?(target.pages[0]?._id||randomId()):randomId();const sectionMap=new Map();const sections=sourcePage.sections.map(section=>{const id=randomId();sectionMap.set(section.id,id);return {title:section.title,description:section.description,layout:section.layout,id,position:section.position,size:section.size}});const items=sourcePage.items.map(sourceItem=>{const item=rewriteEnterprise(JSON.parse(JSON.stringify(sourceItem)));for(const key of ["createdAt","createdBy","modifiedAt","modifiedBy","searchFields","savedSearch"])delete item[key];item.id=randomId();item.sectionId=sectionMap.get(sourceItem.sectionId);item.version=2;if(item.type==="fileFolder"){const link=request.resources?.[item.name];if(link)item.erid=link.kind==="file"?"file_"+link.id:link.id}if(item.type==="form"&&formRef){item.erid="form_"+formRef.id;item.data={...item.data,formId:String(formRef.detail?.form?.id||formRef.summary?.formId||""),fileRequestId:String(formRef.id||""),urlId:String(formRef.detail?.urlId||formRef.detail?.form?.urlId||formRef.summary?.urlId||"")}}if(item.type==="shortcut"){const preferred=item.name.includes("Intake")?request.resources?.["01 - Intake"]:item.name.includes("Workspace")?request.resources?.[source.pages[0].items.find(value=>value.type==="fileFolder"&&value.data?.contentType==="folder")?.name]:item.name.includes("Hub")?Object.values(request.resources||{}).find(value=>value.kind==="hub"):null;if(preferred?.url)item.erid=preferred.url}return item});return {_id:pageId,name:sourcePage.name,sections,items}});let locked=false;try{await meteor("app.lock",[target._id]);locked=true;await meteor("app.update.all",[{_id:target._id,name:request.app.title,description:request.app.description,pages,fromVersion:target.versionNumber}])}finally{if(locked)try{await meteor("app.cancelEdit",[target._id])}catch{}}const verified=await meteor("app.get",[target._id]);result.app={present:true,outcome,title:verified.name,id:verified._id,pageCount:verified.pages?.length||0,sectionCount:(verified.pages||[]).reduce((n,p)=>n+(p.sections?.length||0),0),blockCount:(verified.pages||[]).reduce((n,p)=>n+(p.items?.length||0),0)}}}
return {ok:true,result,hostname:location.hostname,network};
})().then(value=>{window.__boxDispatchPrivateResult=value}).catch(error=>{window.__boxDispatchPrivateResult={ok:false,error:String(error?.stack||error),network:window.__boxDispatchPrivateNetwork}});`, payload)
}
