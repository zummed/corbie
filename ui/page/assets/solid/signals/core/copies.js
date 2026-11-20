/** @internal Runs `fn` with this copy marked as the one running. */
function runAsSolidCopy(n) {
    return n();
}

/** @internal An ownerless component or context read: warns if another copy is running. */ function checkForeignOwnerless() {
    return;
}

export { checkForeignOwnerless, runAsSolidCopy };