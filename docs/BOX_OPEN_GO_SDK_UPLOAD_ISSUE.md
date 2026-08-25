# Multipart upload SDK issue resolved

## Status

Resolved in `github.com/unofficialbox/box-open-go-sdk@v0.4.3`.

Dispatch now uses the generated SDK methods directly:

- `managers.(*UploadsManager).UploadFile`
- `managers.(*UploadsManager).UploadFileVersion`

The temporary Dispatch-owned multipart HTTP transport has been removed.

## Original defect

In v0.4.1, both generated upload methods omitted the supplied file reader from
the multipart request and serialized the request wrapper instead of
`body.Attributes`. The outgoing request therefore lacked a usable `file` part
and used the wrong JSON shape for `attributes`.

## v0.4.3 behavior

The generated methods now:

1. serialize `body.Attributes` as the `attributes` form part;
2. pass `body.File` as the `file` form part;
3. preserve the normal generated-client authentication, retry, and response
   handling path.

Dispatch regression coverage points the generated client at an HTTP test
server, invokes its production upload adapter, parses the multipart request,
and verifies the authorization header, attributes, multipart filename, file
bytes, and returned file ID.
