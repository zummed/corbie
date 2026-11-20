import { NotReadyError } from "./core/error.js";

import "./core/verdict.js";

import "./core/core.js";

import "./core/scheduler.js";

import "./core/effect.js";

// CARVE 4: `flatten` lived in boundaries.ts but is the general children
// resolver the renderers (`web`, `universal`, `solid`'s `children()`) walk
// through — not a boundary feature. Moved verbatim so the boundary carve
// measures boundaries alone.
/**
 * Resolves nested accessors and arrays in a children tree to their rendered
 * values. See the `children()` helper in solid-js for the memoized form.
 */
function flatten(r, t) {
    if (typeof r === "function" && !r.length) {
        if (t?.doNotUnwrap) return r;
        do {
            r = r();
        } while (typeof r === "function" && !r.length);
    }
    if (t?.skipNonRendered && (r == null || r === true || r === false || r === "")) return;
    if (Array.isArray(r)) {
        let e = [];
        if (flattenArray(r, e, t)) {
            return () => {
                let r = [];
                flattenArray(e, r, {
                    ...t,
                    doNotUnwrap: false
                });
                return r;
            };
        }
        return e;
    }
    return r;
}

function flattenArray(r, t = [], e) {
    let o = null;
    let f = false;
    for (let n = 0; n < r.length; n++) {
        try {
            let o = r[n];
            if (typeof o === "function" && !o.length) {
                if (e?.doNotUnwrap) {
                    t.push(o);
                    f = true;
                    continue;
                }
                do {
                    o = o();
                } while (typeof o === "function" && !o.length);
            }
            if (Array.isArray(o)) {
                // OR, don't overwrite: an accessor already pushed under doNotUnwrap
                // still needs the resolving wrapper even when a later sibling
                // fragment contains no functions (#3133).
                f = flattenArray(o, t, e) || f;
            } else if (e?.skipNonRendered && (o == null || o === true || o === false || o === "")) {
                // skip
            } else t.push(o);
        } catch (r) {
            if (!(r instanceof NotReadyError)) throw r;
            o = r;
        }
    }
    if (o) throw o;
    return f;
}

export { flatten };