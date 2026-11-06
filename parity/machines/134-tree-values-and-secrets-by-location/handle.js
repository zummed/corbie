// handle.js: gives back the value it was given, and whether the secret
// TOKEN was sent to it as a handle.
return { hub: input.hub, handle: secrets.get("TOKEN") !== undefined }
