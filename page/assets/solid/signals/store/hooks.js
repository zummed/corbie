// Set before any family is optimistic (`fam.opt`): a call site checks the
// hooks first, so a build that does not import optimistic.ts folds the arm
// away.
let optHooks = null;

function installOptHooks(o) {
    optHooks = o;
}

let affectsHooks = null;

function installAffectsHooks(o) {
    affectsHooks = o;
}

export { affectsHooks, installAffectsHooks, installOptHooks, optHooks };