import{a as e,i as t,n,r,t as i}from"./index-D84KgJjM.js";var a=4e5,o=(e,t)=>{let n=0;for(;n<e.length&&n<t.length&&e[n]===t[n];)n+=1;let r=e.length,i=t.length;for(;r>n&&i>n&&e[r-1]===t[i-1];)--r,--i;let o=e.slice(n,r),s=t.slice(n,i),c=[];if(n>0&&c.push({kind:`equal`,items:e.slice(0,n)}),o.length!==0||s.length!==0){if(o.length===0)c.push({kind:`added`,items:s});else if(s.length===0)c.push({kind:`removed`,items:o});else if(o.length*s.length>a)c.push({kind:`removed`,items:o}),c.push({kind:`added`,items:s});else{let e=o.length,t=s.length,n=Array.from({length:e+1},()=>Array(t+1).fill(0));for(let r=e-1;r>=0;--r)for(let e=t-1;e>=0;--e)n[r][e]=o[r]===s[e]?n[r+1][e+1]+1:Math.max(n[r+1][e],n[r][e+1]);let r=0,i=0,a=(e,t)=>{let n=c[c.length-1];n&&n.kind===e?n.items.push(t):c.push({kind:e,items:[t]})};for(;r<e&&i<t;)o[r]===s[i]?(a(`equal`,o[r]),r+=1,i+=1):n[r+1][i]>=n[r][i+1]?(a(`removed`,o[r]),r+=1):(a(`added`,s[i]),i+=1);for(;r<e;)a(`removed`,o[r]),r+=1;for(;i<t;)a(`added`,s[i]),i+=1}}return r<e.length&&c.push({kind:`equal`,items:e.slice(r)}),c},s=e=>e.split(/(\s+)/).filter(e=>e.length>0),c=e=>{let t=[];for(let n=0;n<e.length;n+=1){let r=e[n],i=e[n-1],a=e[n+1];r.kind===`equal`&&r.text.trim()===``&&i&&a&&i.kind!==`equal`&&i.kind===a.kind?t.push({kind:i.kind,text:r.text}):t.push(r)}let n=[];for(let e of t){let t=n[n.length-1];t&&t.kind===e.kind?t.text+=e.text:n.push({...e})}return n},l=(e,t)=>{let n=o(s(e),s(t)),r=[],i=[];for(let e of n){let t=e.items.join(``);e.kind===`equal`?(r.push({kind:`equal`,text:t}),i.push({kind:`equal`,text:t})):e.kind===`removed`?r.push({kind:`removed`,text:t}):i.push({kind:`added`,text:t})}return{before:c(r),after:c(i)}},u=(e,t)=>{if(e===t)return!0;let n=l(e,t).after.filter(e=>e.kind===`equal`).reduce((e,t)=>e+t.text.length,0),r=Math.max(e.length,t.length);return r>0&&n/r>=.3},d=(e,t)=>{let n=e=>e.length?e.replaceAll(`\r
`,`
`).replaceAll(`\r`,`
`).split(`
`):[],r=o(n(e),n(t)),i=[],a={added:0,removed:0,changed:0},s=1,c=1,d=0;for(;d<r.length;){let e=r[d];if(e.kind===`equal`){for(let t of e.items)i.push({kind:`equal`,before:{number:s,text:t},after:{number:c,text:t}}),s+=1,c+=1;d+=1;continue}let t=r[d+1];if(e.kind===`removed`&&t?.kind===`added`){let n=e.items,r=t.items,o=Math.min(n.length,r.length);for(let e=0;e<o;e+=1){let t=n[e],o=r[e];if(u(t,o)){let e=l(t,o);i.push({kind:`changed`,before:{number:s,text:t,segments:e.before},after:{number:c,text:o,segments:e.after}}),a.changed+=1}else i.push({kind:`removed`,before:{number:s,text:t}}),i.push({kind:`added`,after:{number:c,text:o}}),a.removed+=1,a.added+=1;s+=1,c+=1}for(let e=o;e<n.length;e+=1)i.push({kind:`removed`,before:{number:s,text:n[e]}}),a.removed+=1,s+=1;for(let e=o;e<r.length;e+=1)i.push({kind:`added`,after:{number:c,text:r[e]}}),a.added+=1,c+=1;d+=2;continue}if(e.kind===`removed`)for(let t of e.items)i.push({kind:`removed`,before:{number:s,text:t}}),a.removed+=1,s+=1;else for(let t of e.items)i.push({kind:`added`,after:{number:c,text:t}}),a.added+=1,c+=1;d+=1}let f=[];for(let e=0;e<i.length;e+=1){if(i[e].kind===`equal`)continue;let t=e;for(;e+1<i.length&&i[e+1].kind!==`equal`;)e+=1;f.push({start:t,end:e})}return{rows:i,stats:a,changeRanges:f}},f=e=>{let t=[];return e.added&&t.push(`+${String(e.added)}`),e.removed&&t.push(`−${String(e.removed)}`),e.changed&&t.push(`~${String(e.changed)}`),t.length?t.join(` `):`No changes`},p=`box-diff-viewer`,m=e=>e.replaceAll(`&`,`&amp;`).replaceAll(`<`,`&lt;`).replaceAll(`>`,`&gt;`).replaceAll(`"`,`&quot;`).replaceAll(`'`,`&#39;`),h=`
        [hidden] {
          display: none !important;
        }

        :host {
          display: block;
          color: inherit;
          font: inherit;
        }

        [part="panel"] {
          display: grid;
          gap: ${r.gap};
          padding: ${r.padding};
          border: 1px solid color-mix(in srgb, var(--boe-token-stroke-stroke, #e8e8e8) 82%, transparent);
          border-radius: ${r.radius};
          background: ${r.background};
        }

        [part="header"] {
          display: flex;
          flex-wrap: wrap;
          align-items: center;
          gap: ${r.gap};
        }

        [part="title"] {
          margin: 0;
          font: inherit;
          font-size: 1.05rem;
          font-weight: 700;
          color: var(--boe-token-text-text, #1f1e1b);
        }

        [part="stats"] {
          display: inline-flex;
          padding: 0.2rem 0.55rem;
          border-radius: 999px;
          background: var(--boe-token-surface-item-surface-selected, #f2f7fd);
          color: var(--boe-token-text-text-secondary, #6f6f6f);
          font-size: 0.78rem;
          font-weight: 700;
          font-variant-numeric: tabular-nums;
        }

        [part="nav"] {
          display: inline-flex;
          align-items: center;
          gap: 0.4rem;
          margin-left: auto;
        }

        [part="nav-position"] {
          color: var(--boe-token-text-text-secondary, #6f6f6f);
          font-size: 0.8rem;
          font-variant-numeric: tabular-nums;
        }

        [part="nav-previous"],
        [part="nav-next"] {
          appearance: none;
          font: inherit;
          font-size: 0.8rem;
          font-weight: 600;
          padding: 0.3rem 0.6rem;
          border-radius: ${t.control};
          border: 1px solid var(--boe-token-stroke-stroke, #e8e8e8);
          background: var(--boe-token-surface-surface, #ffffff);
          color: var(--boe-token-text-text, #222222);
          cursor: pointer;
          transition: background ${i.interactive} ${n.standard};
        }

        [part="nav-previous"]:hover:not(:disabled),
        [part="nav-next"]:hover:not(:disabled) {
          background: var(--boe-token-surface-surface-hover, #f4f4f4);
        }

        [part="nav-previous"]:disabled,
        [part="nav-next"]:disabled {
          cursor: not-allowed;
          opacity: 0.5;
        }

        [part="nav-previous"]:focus-visible,
        [part="nav-next"]:focus-visible {
          outline: none;
          box-shadow: 0 0 0 3px color-mix(in srgb, var(--boe-token-surface-surface-brand, #0061d5) 18%, transparent);
        }

        [part="scroller"] {
          overflow-x: auto;
          border: 1px solid color-mix(in srgb, var(--boe-token-stroke-stroke, #e8e8e8) 62%, transparent);
          border-radius: ${t.large};
        }

        [part="table"] {
          width: 100%;
          border-collapse: collapse;
          font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
          font-size: 0.82rem;
          line-height: 1.55;
        }

        [part="column-label"] {
          padding: 0.45rem 0.6rem;
          text-align: left;
          font-family: inherit;
          font-size: 0.74rem;
          font-weight: 700;
          letter-spacing: 0.08em;
          text-transform: uppercase;
          color: var(--boe-token-text-text-secondary, #6f6f6f);
          border-bottom: 1px solid color-mix(in srgb, var(--boe-token-stroke-stroke, #e8e8e8) 62%, transparent);
          background: var(--boe-token-surface-surface-secondary, #fbfbfb);
        }

        [part="line-number"] {
          width: 1%;
          min-width: 2.4rem;
          padding: 0 0.55rem;
          text-align: right;
          vertical-align: top;
          color: color-mix(in srgb, var(--boe-token-text-text-secondary, #6f6f6f) 72%, transparent);
          user-select: none;
          border-right: 1px solid color-mix(in srgb, var(--boe-token-stroke-stroke, #e8e8e8) 46%, transparent);
        }

        [part="cell"] {
          padding: 0 0.6rem;
          vertical-align: top;
          white-space: pre-wrap;
          word-break: break-word;
          color: var(--boe-token-text-text, #1f1e1b);
        }

        [part="row"][data-kind="removed"] [part="cell"][data-side="before"],
        [part="row"][data-kind="changed"] [part="cell"][data-side="before"] {
          background: color-mix(in srgb, var(--boe-token-surface-status-surface-error, #ed3757) 8%, transparent);
        }

        [part="row"][data-kind="added"] [part="cell"][data-side="after"],
        [part="row"][data-kind="changed"] [part="cell"][data-side="after"] {
          background: color-mix(in srgb, var(--boe-token-surface-status-surface-success, #26a27b) 10%, transparent);
        }

        [part="row"][data-active="true"] [part="cell"] {
          box-shadow: inset 3px 0 0 var(--boe-token-surface-surface-brand, #0061d5);
        }

        del {
          text-decoration: line-through;
          text-decoration-thickness: 1px;
          background: color-mix(in srgb, var(--boe-token-surface-status-surface-error, #ed3757) 22%, transparent);
          color: inherit;
        }

        ins {
          text-decoration: none;
          background: color-mix(in srgb, var(--boe-token-surface-status-surface-success, #26a27b) 26%, transparent);
          color: inherit;
        }

        [part="empty"] {
          padding: ${r.padding};
          border-radius: ${t.large};
          border: 1px dashed color-mix(in srgb, var(--boe-token-stroke-stroke, #e8e8e8) 70%, transparent);
          color: var(--boe-token-text-text-secondary, #6f6f6f);
        }
      `,g=class extends e{static tagName=p;static get observedAttributes(){return[`after-label`,`after-text`,`before-label`,`before-text`,`heading`,`mode`]}titleEl;statsEl;navEl;navPositionEl;navPreviousEl;navNextEl;scrollerEl;emptyEl;result=null;resultSignature=``;activeChangeIndex=-1;get heading(){return this.getAttribute(`heading`)??`Comparison`}set heading(e){this.setAttribute(`heading`,e)}get beforeText(){return this.getAttribute(`before-text`)??``}set beforeText(e){this.setAttribute(`before-text`,e)}get afterText(){return this.getAttribute(`after-text`)??``}set afterText(e){this.setAttribute(`after-text`,e)}get beforeLabel(){return this.getAttribute(`before-label`)??`Original`}set beforeLabel(e){this.setAttribute(`before-label`,e)}get afterLabel(){return this.getAttribute(`after-label`)??`Revised`}set afterLabel(e){this.setAttribute(`after-label`,e)}get mode(){return this.getAttribute(`mode`)===`inline`?`inline`:`split`}set mode(e){this.setAttribute(`mode`,e)}get diff(){return this.result}goToChange(e){let t=this.result?.changeRanges??[];if(!t.length)return;let n=Math.max(0,Math.min(t.length-1,e));this.activeChangeIndex=n,this.isRendered&&this.update();let r=t[n];this.scrollerEl.querySelector(`[part="row"][data-row-index="${String(r?.start??0)}"]`)?.scrollIntoView?.({block:`center`}),this.dispatchEvent(new CustomEvent(`change-focused`,{bubbles:!0,composed:!0,detail:{index:n,total:t.length}}))}lineHtml(e){return e?e.segments?e.segments.map(e=>{let t=m(e.text);return e.kind===`removed`?`<del>${t}</del>`:e.kind===`added`?`<ins>${t}</ins>`:t}).join(``):m(e.text):``}rowHtml(e,t){let n=this.result?.rows[e];if(!n)return``;let r=`part="row" data-kind="${n.kind}" data-row-index="${String(e)}" data-active="${t?`true`:`false`}"`;if(this.mode===`inline`){let e=[];return n.before&&n.kind!==`equal`&&e.push(`
          <tr ${r}>
            <td part="line-number">${String(n.before.number)}</td>
            <td part="line-number"></td>
            <td part="cell" data-side="before">${this.lineHtml(n.before)}</td>
          </tr>
        `),n.after&&e.push(`
          <tr ${r}>
            <td part="line-number">${n.kind===`equal`&&n.before?String(n.before.number):``}</td>
            <td part="line-number">${String(n.after.number)}</td>
            <td part="cell" data-side="${n.kind===`equal`?`context`:`after`}">${this.lineHtml(n.after)}</td>
          </tr>
        `),e.join(``)}return`
      <tr ${r}>
        <td part="line-number">${n.before?String(n.before.number):``}</td>
        <td part="cell" data-side="before">${this.lineHtml(n.before)}</td>
        <td part="line-number">${n.after?String(n.after.number):``}</td>
        <td part="cell" data-side="after">${this.lineHtml(n.after)}</td>
      </tr>
    `}rebuildTable(){let e=this.result?.rows??[],t=this.activeChangeIndex>=0?this.result?.changeRanges[this.activeChangeIndex]:void 0,n=e=>!!t&&e>=t.start&&e<=t.end,r=this.mode===`inline`?`<tr>
            <th part="column-label" scope="col" colspan="2">Line</th>
            <th part="column-label" scope="col">${m(`${this.beforeLabel} → ${this.afterLabel}`)}</th>
          </tr>`:`<tr>
            <th part="column-label" scope="col" colspan="2">${m(this.beforeLabel)}</th>
            <th part="column-label" scope="col" colspan="2">${m(this.afterLabel)}</th>
          </tr>`;this.scrollerEl.innerHTML=`
      <table part="table" aria-label="${m(`${this.heading}: ${this.beforeLabel} vs ${this.afterLabel}`)}">
        <thead>${r}</thead>
        <tbody>${e.map((e,t)=>this.rowHtml(t,n(t))).join(``)}</tbody>
      </table>
    `}renderTemplate(){this.shadowRoot&&(this.shadowRoot.innerHTML=`
      <style>${h}</style>
      <section part="panel">
        <header part="header">
          <h2 part="title"></h2>
          <span part="stats"></span>
          <span part="nav" hidden>
            <button type="button" part="nav-previous" aria-label="Previous change">‹ Prev</button>
            <span part="nav-position" aria-live="polite"></span>
            <button type="button" part="nav-next" aria-label="Next change">Next ›</button>
          </span>
        </header>
        <div part="scroller" tabindex="0"></div>
        <div part="empty" hidden>Nothing to compare.</div>
      </section>
    `,this.titleEl=this.shadowRoot.querySelector(`[part="title"]`),this.statsEl=this.shadowRoot.querySelector(`[part="stats"]`),this.navEl=this.shadowRoot.querySelector(`[part="nav"]`),this.navPositionEl=this.shadowRoot.querySelector(`[part="nav-position"]`),this.navPreviousEl=this.shadowRoot.querySelector(`[part="nav-previous"]`),this.navNextEl=this.shadowRoot.querySelector(`[part="nav-next"]`),this.scrollerEl=this.shadowRoot.querySelector(`[part="scroller"]`),this.emptyEl=this.shadowRoot.querySelector(`[part="empty"]`))}setupListeners(){this.navPreviousEl.addEventListener(`click`,()=>{this.goToChange(this.activeChangeIndex<=0?0:this.activeChangeIndex-1)}),this.navNextEl.addEventListener(`click`,()=>{this.goToChange(this.activeChangeIndex+1)})}update(){if(!this.scrollerEl)return;this.titleEl.textContent=this.heading;let e=this.beforeText,t=this.afterText,n=e.length>0||t.length>0;this.emptyEl.hidden=n,this.scrollerEl.hidden=!n;let r=[this.mode,String(e.length),String(t.length),e,t].join(`\0`);if(r!==this.resultSignature)this.resultSignature=r,this.result=n?d(e,t):null,this.activeChangeIndex=-1,n?this.rebuildTable():this.scrollerEl.innerHTML=``;else if(this.result){let e=this.activeChangeIndex>=0?this.result.changeRanges[this.activeChangeIndex]:void 0;this.scrollerEl.querySelectorAll(`[part="row"]`).forEach(t=>{let n=Number(t.dataset.rowIndex);t.dataset.active=e&&n>=e.start&&n<=e.end?`true`:`false`})}let i=this.scrollerEl.querySelector(`[part="table"]`);if(i){i.setAttribute(`aria-label`,`${this.heading}: ${this.beforeLabel} vs ${this.afterLabel}`);let e=i.querySelectorAll(`[part="column-label"]`);this.mode===`inline`?e[1].textContent=`${this.beforeLabel} → ${this.afterLabel}`:(e[0].textContent=this.beforeLabel,e[1].textContent=this.afterLabel)}let a=this.result?.stats??{added:0,removed:0,changed:0};this.statsEl.textContent=f(a);let o=this.result?.changeRanges.length??0;this.navEl.hidden=o===0,this.navPositionEl.textContent=this.activeChangeIndex>=0?`Change ${String(this.activeChangeIndex+1)} of ${String(o)}`:`${String(o)} changes`,this.navPreviousEl.disabled=this.activeChangeIndex<=0,this.navNextEl.disabled=o===0||this.activeChangeIndex>=o-1}};g.register();export{g as DiffViewer};