// handle.js: gives back its input, and whether the secret TOKEN was sent to
// it as a handle.
return { ...input, handle: secrets.get("TOKEN") !== undefined }
