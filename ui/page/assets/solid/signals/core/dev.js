import { context, untrack } from "./core.js";

import { NotReadyError } from "./error.js";

const diagnosticListeners = new Set;

const diagnosticCaptures = new Set;

let diagnosticSequence = 0;

/**
 * Registers the console footer appended to the first console report of
 * each diagnostic code — a discovery pointer to deeper guidance. Reported
 * events carry it as trailing lines of the same console entry; events that
 * surface as a thrown error instead get it as a follow-up line. Returning
 * undefined for an event suppresses the footer. Passing undefined
 * unregisters and resets the once-per-code memory.
 *
 * @internal A seam for `solid-js`, which owns the repair skill the footer
 * names and installs it from both of its entries; not part of `DEV`. No-op
 * outside dev builds, where nothing reports to the console.
 */ function setConsoleFooter(e) {
    return;
}

/** Events built for an excluded subject: never delivered, never reported. */ const suppressedEvents = new WeakSet;

function isExcluded(e) {
    return false;
}

const DEV = undefined;

/**
 * Root-first names of the owners enclosing `subject` (inclusive when the
 * subject is itself a named owner). Signals hop to their registering owner
 * (`_owner`, set by registerGraph). Unnamed owners are skipped so the path
 * reads as the component tree plus the scope: `<App> › <TodoRow> › effect`.
 * Public as `OBSERVE.ownerPath`; the core's own sites import it directly.
 */ function ownerPath(e) {
    if (!e) return undefined;
    let t = "_parent" in e ? e : e.H ?? null;
    const n = [];
    for (;t !== null; t = t._parent) {
        const e = t._name;
        if (typeof e === "string" && e.length) n.push(e);
    }
    return n.length ? n.reverse() : undefined;
}

/**
 * The innermost source location on the subject's chain: the subject's own
 * `_loc` (a signal or computation created with `{ loc }`), else the first
 * `_loc` up the owners — a signal hops to its registering owner, then
 * `_parent` upward (a component root carries its JSX site). The same walk
 * `ownerPath` makes; `undefined` when nothing on the chain was located.
 */ function locationOf(e) {
    // Owners have a `_parent` key (null on a root); a signal has none and hops
    // to its registering `_owner` instead.
    for (let t = e; t; t = t._parent !== undefined ? t._parent : t.H) if (t._loc) return t._loc;
    return undefined;
}

function emitDiagnostic(e, t = context) {
    const n = {
        sequence: ++diagnosticSequence,
        ...e
    };
    // The observer's own subtree: build the entry (the caller may throw its
    // message) but tell nobody.
        if (isExcluded()) {
        suppressedEvents.add(n);
        return n;
    }
    if (n.ownerPath === undefined) {
        const e = ownerPath(t);
        if (e) n.ownerPath = e;
    }
    if (n.loc === undefined) {
        const e = locationOf(t);
        if (e !== undefined) n.loc = e;
    }
    const o = t ?? undefined;
    if (o !== undefined) eventSubjects.set(n, o);
    for (const e of diagnosticListeners) e(n, o);
    for (const e of diagnosticCaptures) e.push(n);
    return n;
}

/**
 * The subject each emitted event was about, for the console step, which
 * runs after `emitDiagnostic` returned and can show what the node knows: a
 * rendering runtime may stamp a binding effect with the DOM element it
 * writes (`_devElement`), and a live element reference beside the message
 * is the most addressable pointer a console can print. Listeners get the
 * subject as their second argument instead; the map exists only to carry
 * it from `emitDiagnostic` to `reportDiagnostic` across the call site's
 * `reportDiagnostic(emitDiagnostic(…))`. Weak, keyed by the entry.
 */ const eventSubjects = new WeakMap;

/**
 * The console face of a diagnostic — ONE entry per finding: the message, the
 * owner path (`in <App> › <TodoRow> › effect`) so a human can locate it, the
 * source location (`at src/App.tsx:18:3` — see `locationLine`) when a node on
 * the chain was compiled with one, the once-per-code footer as trailing
 * lines, and — when the subject is a binding effect the rendering runtime
 * tagged — the element it writes, as a second console argument (hover
 * highlights it, click jumps to Elements).
 * Severity picks the console method. Call sites report the entry
 * `emitDiagnostic` returned so the structured and console channels never
 * disagree. Dev-tier: in an observe build this is a no-op, so wiring paths
 * that both emit and report (graph-size warnings) reach the channel only —
 * production observability never writes to the console.
 */ function reportDiagnostic(e) {
    return;
}

const thrownReads = new WeakMap;

const caughtReadReported = new WeakSet;

/** The runtime's own catch swallowed (or re-routed) the last throw into the
 * running pass: it is the mechanism, not a user catch. Dev only.
 * @internal Also solid-js's: its hydration trace runs a compute whose
 * throws the tracer, not user code, observes. */ function forgetThrownRead() {
    return;
}

/** Flight shape, as `handleAsync` probes it (untracked: a store proxy's
 * `then` is a read). */ function isFlight(e) {
    if (typeof e !== "object" || e === null) return false;
    return untrack(() => !!e[Symbol.asyncIterator] || typeof e.then === "function");
}

/** `el`'s `_fn` returned `result`. A throw still recorded against the pass
 * was caught inside the body — unless `result` is the flight the body
 * became (an `async` function's rejected promise where a sync body would
 * have thrown): then the flight's settle judges it (`caughtReadFlightSettled`).
 * Called before `handleAsync` sees the result: a pending flight throws
 * NotReady out of it, and the verdict must already be pinned to the flight. */ function checkCaughtRead(e, t) {
    const n = thrownReads.get(e);
    if (n === undefined) return;
    if (n.flight === undefined && isFlight(t)) {
        n.flight = t;
        return;
    }
    if (n.flight !== undefined) return;
    thrownReads.delete(e);
    reportCaughtRead(e, n);
}

function reportCaughtRead(e, t) {
    if (caughtReadReported.has(e)) return;
    caughtReadReported.add(e);
    const n = e._name;
    const o = t.source._name;
    const r = n ? `"${n}"` : "A computation";
    const i = o ? `"${o}"` : "a reactive value";
    const a = t.error instanceof NotReadyError ? `[CAUGHT_REACTIVE_READ] ${r} caught the not-ready throw of ${i} and completed with a value. ` + `A reactive read is not an await: the throw is how the read suspends to a <Loading> boundary or holds the change, ` + `and when the source changes later this computation is marked pending and not re-run, so the catch will not fire again. ` + `Branch on readiness with isPending(), show ahead with latest(), or let the read suspend.` : `[CAUGHT_REACTIVE_READ] ${r} caught the error of ${i} and completed with a value. ` + `An error is the source's status, not a throw this computation owns: the computation is errored with it regardless, ` + `and today's runtime may complete with the caught value until the source is retried, ` + `when the error reaches <Errored> (or halts) and the catch does not fire again. ` + `Handle errors with <Errored> or createEffect's error option.`;
    reportDiagnostic(emitDiagnostic({
        code: "CAUGHT_REACTIVE_READ",
        kind: "async",
        severity: "warn",
        message: a,
        ownerId: e.id,
        ownerName: n,
        nodeName: o
    }, e));
}

export { DEV, checkCaughtRead, emitDiagnostic, forgetThrownRead, isExcluded, ownerPath, reportDiagnostic, setConsoleFooter };