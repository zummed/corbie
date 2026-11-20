import { isWrappable, ownEnumerableKeys } from "./types.js";

const DELETE = Symbol(0);

function isPrototypePollutionKey(t) {
    return t === "__proto__" || t === "constructor" || t === "prototype";
}

function updatePath(t, e, o = 0) {
    let r, n = t;
    if (o < e.length - 1) {
        r = e[o];
        const i = typeof r;
        const f = Array.isArray(t);
        if (i === "string" && isPrototypePollutionKey(r)) return;
        if (Array.isArray(r)) {
            for (let n = 0; n < r.length; n++) {
                e[o] = r[n];
                updatePath(t, e, o);
            }
            e[o] = r;
            return;
        } else if (f && i === "function") {
            for (let n = 0; n < t.length; n++) {
                if (r(t[n], n)) {
                    e[o] = n;
                    updatePath(t, e, o);
                }
            }
            e[o] = r;
            return;
        } else if (f && i === "object") {
            const {from: n = 0, to: i = t.length - 1, by: f = 1} = r;
            for (let r = n; r <= i; r += f) {
                e[o] = r;
                updatePath(t, e, o);
            }
            e[o] = r;
            return;
        } else if (o < e.length - 2) {
            updatePath(t[r], e, o + 1);
            return;
        }
        n = t[r];
    }
    let i = e[e.length - 1];
    if (typeof i === "function") {
        i = i(n);
        if (i === n) return;
    }
    if (r === undefined && i == undefined) return;
    if (i === DELETE) {
        delete t[r];
    } else if (r === undefined || isWrappable(n) && isWrappable(i) && !Array.isArray(i)) {
        const e = r !== undefined ? t[r] : t;
        const o = ownEnumerableKeys(i);
        for (let t = 0; t < o.length; t++) {
            const r = o[t];
            if (typeof r === "string" && isPrototypePollutionKey(r)) continue;
            const n = Object.getOwnPropertyDescriptor(i, r);
            if (n.get || n.set) Object.defineProperty(e, r, n); else e[r] = n.value;
        }
    } else {
        t[r] = i;
    }
}

/**
 * Path-based setter helper for `createStore`. Call `storePath(...path, value)`
 * to produce a draft-mutating function suitable for passing to `setStore`.
 *
 * The canonical setter form in Solid 2.0 is the draft-mutating callback
 * (`setStore(s => { s.user.name = "Ada"; })`). `storePath` is a backwards-
 * compatibility helper for users porting from Solid 1.x's
 * `setStore("user", "name", "Ada")` style — it's optional and you can mix the
 * two styles freely.
 *
 * Path parts can be:
 * - a single key — `"user"`, `0`
 * - an array of keys — `[0, 1, 2]`
 * - a range over an array — `{ from?, to?, by? }`
 * - a filter `(item, index) => boolean` for arrays
 *
 * The final argument is the new value or an updater `(prev) => next`. Use
 * `storePath.DELETE` to remove a property.
 *
 * @example
 * ```ts
 * const [state, setState] = createStore({ user: { name: "Ada" }, todos: [] });
 *
 * setState(storePath("user", "name", "Grace"));
 * setState(storePath("todos", t => !t.done, "done", true)); // mark all undone as done
 * setState(storePath("user", "nickname", storePath.DELETE));
 * ```
 */ const storePath = /* @__PURE__ */ Object.assign(function storePath(...t) {
    return e => {
        updatePath(e, t);
    };
}, {
    DELETE: DELETE
});

export { storePath };