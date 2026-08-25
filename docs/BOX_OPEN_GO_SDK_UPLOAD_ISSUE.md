# Generated upload methods omit the file stream from multipart requests

## Summary

In `github.com/unofficialbox/box-open-go-sdk@v0.4.1`, both generated upload
methods build a multipart request with a `nil` file reader. The request contains
an `attributes` part but no `file` part, even when the caller populates
`CreateFileContentRequest.File` or `CreateFileIdContentRequest.File`.

The same methods marshal the entire request wrapper for the `attributes` part,
producing `{"attributes": {...}}`; Box expects the value of `body.Attributes`
directly.

Affected generated methods:

- `managers.(*UploadsManager).UploadFile`
- `managers.(*UploadsManager).UploadFileVersion`

## Current generated code

```go
attributes, err := json.Marshal(body)
if err != nil {
    return nil, err
}
req = gantryruntime.WithMultipartBody(req, attributes, "file", nil)
```

`body.File` is never passed to `WithMultipartBody`.

## Reproduction

```go
source, _ := os.Open("example.txt")
defer source.Close()

result, err := client.Uploads.UploadFile(ctx, &schemas.CreateFileContentRequest{
    Attributes: schemas.PostFileContentAttributes{
        Name: "example.txt",
        Parent: schemas.AttributesParent{Id: "12345"},
    },
    File: source,
}, nil)
```

Inspecting the outgoing multipart body shows:

- an `attributes` part containing a nested `attributes` object;
- no `file` form part;
- the supplied file bytes are never read.

In a live Dispatch deployment, the request returned without a transport error,
but the decoded `Files` response contained no entry ID and the file did not
appear in the target folder.

## Expected behavior

The multipart request should contain:

1. an `attributes` part whose JSON value is `body.Attributes`;
2. a `file` part whose reader is `body.File` and whose filename is the
   requested file name.

## Suggested generated output

For `UploadFile`:

```go
attributes, err := json.Marshal(body.Attributes)
if err != nil {
    return nil, err
}
req = gantryruntime.WithMultipartBody(req, attributes, body.Attributes.Name, body.File)
```

Use the equivalent change for `UploadFileVersion`.

## Regression test

Point the generated client at an `httptest.Server`, call both upload methods,
parse the multipart request, and assert:

- `FormValue("attributes")` decodes directly to the expected Box attributes;
- `FormFile("file")` exists;
- reading the file part returns the exact source bytes.

Because the file is already represented as `io.Reader` and tagged `json:"-"`,
the generator should treat multipart request schemas specially rather than
marshalling the whole request wrapper.
