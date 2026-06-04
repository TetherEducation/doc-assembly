# Doc Assembly

This context names the document assembly concepts that must stay stable across the core library and host applications that embed it.

## Language

**Legacy Document Proxy**:
A compatibility capability for documents created outside the current doc-assembly document lifecycle. It lets a host application centralize contract access through doc-assembly without creating a separate service.
_Avoid_: custom routes framework, alternate document access path, doc-assembly document bypass

**Legacy Document Handler**:
The host application's implementation for resolving a Legacy Document request. It owns authentication, authorization, request interpretation, legacy lookup, and the JSON access representation returned to the caller.
_Avoid_: doc-assembly authorization, core document resolver

**Legacy Document**:
A document owned by a system that existed before or outside the current doc-assembly document lifecycle. A Legacy Document can be exposed through the Legacy Document Proxy, but it does not become a Doc Assembly Document.
_Avoid_: doc-assembly document, managed document

**Doc Assembly Document**:
A document created, stored, rendered, signed, or viewed through the current doc-assembly document lifecycle. It must be accessed through the standard doc-assembly document, signing, and read-only view flows.
_Avoid_: legacy document, external document

## Example Dialogue

Developer: "Can we use the Legacy Document Proxy to fetch an old contract from our previous storage system?"

Domain Expert: "Yes, if that contract was not created by the current doc-assembly lifecycle."

Developer: "Who decides whether that old contract can be returned?"

Domain Expert: "The Legacy Document Handler decides that; doc-assembly only provides the compatibility entry point and required workspace/environment context."

Developer: "Can we use it to download a doc-assembly document without going through the read-only view flow?"

Domain Expert: "No. Doc Assembly Documents stay on the standard document access paths; the proxy only exists for legacy documents."
