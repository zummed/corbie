import {
  s as setVerdict,
  c as creatorPass,
  t as tracking,
  a as adopter,
  f as flushTransaction,
  R as REACTIVE_JOINED,
  C as CONFIG_VERDICT,
  S as STATUS_UNINITIALIZED,
  b as REACTIVE_PROBED,
  d as REACTIVE_WINDOW,
  W as WINDOW_KINDS,
  e as creatorReads,
  g as setAdopter,
  h as dropFlushTx,
  i as REACTIVE_MISSED_WAKE,
  N as NotReadyError,
  j as context,
  k as REACTIVE_PROBE_UNANSWERED,
  v as verdict,
  l as forgetThrownRead,
  m as CONFIG_CHILDREN_FORBIDDEN,
  p as prepareComputed,
  n as link,
  o as NOT_PENDING,
  q as clock,
  r as ext,
  u as CONFIG_HELD,
  w as txOf,
  x as pullComputed,
  y as snapshotCaptureActive,
  z as CONFIG_IN_SNAPSHOT_SCOPE,
  A as STATUS_ERROR,
  B as NO_SNAPSHOT,
  D as REACTIVE_SNAPSHOT_STALE,
  G as GlobalQueue,
  E as CONFIG_OVERRIDE,
  F as CONFIG_GUESS,
  H as STATUS_PENDING,
  I as latestOf,
  J as screenOf,
  K as passLane,
  L as sameLane,
  M as resolveTx,
  O as staleReader,
  P as strictRead,
  Q as strictReadActive,
  T as warnStrictReadUntracked,
  U as EFFECT_RENDER,
  V as blocked,
  X as keptGuess,
  Y as globalQueue,
  Z as joinFuture,
  _ as joinPass,
  $ as CONFIG_STAGED_ERROR,
  a0 as markLateLinker,
  a1 as spectating,
  a2 as REACTIVE_DISPOSED,
  a3 as installVerdictHooks,
  a4 as unflushedValue,
  a5 as REACTIVE_LANE_READ,
  a6 as ahead,
  a7 as REACTIVE_STAGED_READ,
  a8 as CONFIG_HELD_FRAME,
  a9 as bornInto,
  aa as setPassLane,
  ab as verdictLane,
  ac as isSame,
  ad as stagedReaders,
  ae as guessing,
  af as pendingNodes,
  ag as REACTIVE_MANUAL_WRITE,
  ah as heldRewrites,
  ai as isThenable,
  aj as REACTIVE_FRAME_READ,
  ak as enqueueSub,
  al as isVerdict,
  am as deleteFromHeap,
  an as dirtyQueue,
  ao as releaseFlightTeardown,
  ap as handleAsync,
  aq as notifyStatus,
  ar as CONFIG_ASYNC,
  as as CONFIG_OWN_PROBE,
  at as CONFIG_FRAME,
  au as waiting,
  av as CONFIG_STAGED,
  aw as heldMountUnder,
  ax as queuePendingNode,
  ay as insertSubs,
  az as CONFIG_REDERIVE,
  aA as EFFECT_TRACKED,
  aB as settlePendingSource,
  aC as REACTIVE_REASK,
  aD as CONFIG_INPUTS_PUBLISHED,
  aE as tickBase,
  aF as REACTIVE_IN_HEAP,
  aG as REACTIVE_DIRTY,
  aH as REACTIVE_CHECK,
  aI as list,
  aJ as REACTIVE_LANE_DIRTY,
  aK as schedule,
  aL as covers,
  aM as newTransaction,
  aN as attrHooks,
  aO as enterAction,
  aP as setEffectCallback,
  aQ as exitAction,
  aR as actionDepth,
  aS as flush,
  aT as nextQuestion,
  aU as setQuestion,
  aV as question,
  aW as inRefCallback,
  aX as emitDiagnostic,
  aY as enterCallback,
  aZ as exitCallback,
  a_ as getOwner,
  a$ as inEffectCallback,
  b0 as effectCallbackEvent,
  b1 as effectCallbackOwner,
  b2 as setExternalHooks,
  b3 as signal,
  b4 as setSignal,
  b5 as cleanup,
  b6 as read,
  b7 as DEV$1,
  b8 as CONFIG_TRANSPARENT,
  b9 as NoOwnerError,
  ba as ContextNotFoundError,
  bb as createEffectNode,
  bc as recompute,
  bd as applyStaged,
  be as EFFECT_USER,
  bf as reportDiagnostic,
  bg as computed,
  bh as staleValues,
  bi as CONFIG_AUTO_DISPOSE,
  bj as heldRunHook,
  bk as attachHook,
  bl as unwrapStatusError,
  bm as setStrictRead,
  bn as queueEvent,
  bo as StatusError,
  bp as trimStaleDeps,
  bq as haltReactivityAt,
  br as notifyEpoch,
  bs as strictReadEvent,
  bt as setTrackedQueueCallback,
  bu as setEffectStatusNotify,
  bv as queueEffectRun,
  bw as _hitUnhandledAsync,
  bx as resetUnhandledAsync,
  by as ROOT_ERROR_HOOK,
  bz as CONFIG_SLOT_NODE,
  bA as devWarnOptimisticWhileLoading,
  bB as armGuessing,
  bC as armGuessed,
  bD as installGuessLayer,
  bE as installGuessHooks,
  bF as rerun,
  bG as covered,
  bH as setLaneValue,
  bI as lanes,
  bJ as marked$1,
  bK as commitPendingNode,
  bL as landBeneath,
  bM as ownFlights,
  bN as guessFlights,
  bO as underOf,
  bP as markJudged,
  bQ as merge$1,
  bR as readGuesses,
  bS as dissolveLane,
  bT as holdNode,
  bU as newLane,
  bV as inFlight$1,
  bW as laneDirty,
  bX as registerGraph,
  bY as runWithOwner,
  bZ as setMemo,
  b_ as $REFRESH,
  b$ as markRefresh,
  c0 as createRoot,
  c1 as getObserver,
  c2 as dispose,
  c3 as TimeoutError,
  c4 as untrack,
  c5 as CONFIG_AUTHORITATIVE,
  c6 as truthRead,
  c7 as truthConfirmed,
  c8 as passTx,
  c9 as CONFIG_FRESH_READ,
  ca as createOwner,
  cb as writeHook,
  cc as REACTIVE_WRITE_IN_OWNED_SCOPE_SIGNAL_MESSAGE,
  cd as ownedScopeWriteMessage,
  ce as isEqual,
  cf as CONFIG_MANUAL_WRITE,
  cg as laneStageHook,
  ch as captureWriteSnapshot,
  ci as slotSignal,
  cj as notePromotedWrite,
  ck as observeFlightHook,
  cl as CONFIG_PROMOTED,
  cm as joinPassTx,
  cn as stagedScreen,
  co as stagedRead,
  cp as setSlotUnobserved,
  cq as installStoreHooks,
  cr as disposalDepth,
  cs as insertIntoHeap,
  ct as registerDevStoreSlotWriter,
  cu as checkForeignRead,
  cv as asyncTailFlights,
  cw as untrackDepth,
  cx as checkPostAwaitRead,
  cy as REACTIVE_RECOMPUTING_DEPS,
  cz as devRead,
  cA as registerStoreTools,
  cB as isDisposed,
  cC as laneServedHook,
  cD as landBySetter,
  cE as addPendingSource,
  cF as propagateStatus,
  cG as forEachDependent,
  cH as devGuardOptimisticBeforeLoad,
  cI as joinPassAt,
  cJ as SUPPORTS_PROXY,
  cK as CONFIG_KEY_BAG,
  cL as releaseMarksHook,
  cM as installMarkHooks,
  cN as CONFIG_LAGS,
  cO as onSupersede,
  cP as haltReactivity,
  cQ as swapHook,
  cR as heldUnder,
  cS as unhold,
  cT as REACTIVE_ZOMBIE,
  cU as installBoundaryHooks,
  cV as CAUGHT_REARMED,
  cW as OBSERVE$1
} from "./dev-shared.js";
export {
  cX as checkForeignOwnerless,
  cY as clearSnapshots,
  cZ as enforceLoadingBoundary,
  c_ as getNextChildId,
  c$ as heldRead,
  d0 as isHidden,
  d1 as markSnapshotScope,
  d2 as peekNextChildId,
  d3 as releaseSnapshotScope,
  d4 as resetErrorHalt,
  d5 as runAsSolidCopy,
  d6 as runRefCallback,
  d7 as setConsoleFooter,
  d8 as setSnapshotCapture,
  d9 as setSwapHook,
  da as setWriteHook,
  db as strictReadTracked
} from "./dev-shared.js";

// Verdicts: `latest(fn)` and `isPending(fn)`, the display-ahead reads (F-6,
// D-22, D-23, D-25). A verdict reader runs in its holder's verdict lane:
// shown now, re-derived at the landing.
/** The seam decides this reader's answer: it re-runs if the flush parks. */
function watchVerdict(c, unanswered = false) {
  if (unanswered) c._flags |= REACTIVE_PROBE_UNANSWERED;
  stagedReaders.push(c);
}
/** Seat the pass in the holder's verdict lane, unless it is already lane
 * work or the hold's own work (D-25: a pass the hold built reads plainly). */
function route(c, t) {
  if (
    !passLane &&
    !(c._flags & (REACTIVE_JOINED | REACTIVE_STAGED_READ)) &&
    !(c._config & CONFIG_HELD_FRAME && bornInto(c, resolveTx(t)))
  )
    setPassLane(verdictLane(t));
  c._flags |= REACTIVE_LANE_READ;
}
/** A verdict reader's plain read of a flight, served the committed value,
 * still holds the frame on that flight (D-23: `[isPending(x), x()]` holds
 * like `x()`; D-25). A flight nothing held yet is queued here, so the hold
 * has an entry for it (#3458; without it the reader re-derived every flush,
 * fuzzer F12). True when the reader is served the committed value: not
 * when that would tear against inputs already on screen (F-1, #3305), nor
 * for a reader born into the flight's hold (#4010). */
function observeFlight(c, el) {
  // Either read order holds the frame, a routed render effect too (D-25, S-52).
  if (el._config & CONFIG_INPUTS_PUBLISHED || (el._config & CONFIG_HELD && bornInto(c, txOf(el))))
    return false;
  // A derived store's derive with no reader is no display (F-3).
  if (globalQueue._running && c._fam?.live.size !== 0) {
    joinFuture(null);
    if (!(el._config & CONFIG_HELD)) queuePendingNode(el);
    ahead.splice(ahead.indexOf(c) >>> 0, 1);
    verdictRead(c, flushTransaction, true);
  }
  return true;
}
/** Which window is open, and whether an `isPending` probe found anything. */
let latestActive = false;
let probing = false;
let probeFound = false;
/** A verdict read of a held node. A probe re-runs at the landing because
 * nothing notifies its flip; `latest` needs no re-run, the landing reaches
 * it through its link. */
function verdictRead(c, t, plain) {
  route(c, t);
  if (plain) staleReader(c, t);
  else if (probing && t._reruns[t._reruns.length - 1] !== c) t._reruns.push(c);
}
/** A verdict read of a node staged this flush but not held: whether it will
 * be held is the seam's decision. Before anything has joined a hold, the
 * reader re-runs if the flush parks, so a plain sync write never reads as
 * pending (F-9). `latest` opens no transaction for its answer (#4365). */
function provisionalVerdict(c, notFinal) {
  if (flushTransaction) {
    if (probing && notFinal) probeFound = true;
    verdictRead(c, flushTransaction);
    if (probing && c._flags & REACTIVE_STAGED_READ) watchVerdict(c);
    return;
  }
  // Unanswered: a later plain read may route this pass, and the seam must
  // not take that as the probe's answer.
  watchVerdict(c, !probing || notFinal);
}
/** A re-ask of the answered question is quiet (F-9); so is a node pending
 * only through quiet flights. */
function quietPending(el) {
  const x = el._x;
  if (!x) return false;
  if (x._pendingSources) {
    for (const source of x._pendingSources) if (!source._x?._reask) return false;
    return true;
  }
  return x._reask;
}
/** `isPending` of a held node with a committed value (F-9, D-23): a flight
 * is not final unless quiet; a staging is not final unless it is a quiet
 * re-ask, equal to the shown (non-error) value, or read inside `latest`,
 * whose view is the proposal. Truthy when not final. */
function heldNotFinal(owner) {
  if ((owner._statusFlags & STATUS_PENDING) !== 0 && (!quietPending(owner) || heldBeneath(owner)))
    return true;
  return (
    owner._pendingValue !== NOT_PENDING &&
    !latestActive &&
    !owner._x._reask &&
    (owner._x._shownError || !owner._equals || !owner._equals(owner._value, owner._pendingValue))
  );
}
/** While a re-ask flies, the error is still `latest`'s newest answer (D-22, D-41). */
function aheadError(el) {
  const e = el._statusFlags & STATUS_PENDING && (el._x?._aheadError || el._x?._shownError);
  if (e) throw e;
}
/** A source holds an answer to a new question, so a quiet flight does not make `el` final (F-9). */
function heldBeneath(el) {
  for (let d = el._deps; d !== null; d = d._nextDep) {
    const dep = d._dep;
    if (
      dep._config & CONFIG_HELD &&
      dep._pendingValue !== NOT_PENDING &&
      !dep._x?._reask &&
      !isSame(dep, dep._value, dep._pendingValue)
    )
      return true;
  }
  return false;
}
function pendingVerdict(c, el) {
  if (probing && (!quietPending(el) || heldBeneath(el))) probeFound = true;
  if (c && tracking) {
    if (el._config & CONFIG_HELD) verdictRead(c, txOf(el));
    else if (globalQueue._running) {
      // `latest` of a flight is the committed value in every world (D-22),
      // so the reader joins the verdict lane now; a seam re-derive would ask
      // its question twice (F-9, #4397). A probe in a transaction only
      // `latest` opened takes no landing re-run, which would reopen it every
      // flush (#4661).
      if (!probing) {
        c._flags & REACTIVE_LANE_READ || ahead.push(c);
        flushTransaction || (joinFuture(null), (flushTransaction.latest = 1));
      } else if (flushTransaction?.latest) return route(c, flushTransaction);
      provisionalVerdict(c, false); // decided above
    }
  }
}
/** Mark the pass entering a window as a verdict reader. An untracked window
 * is a snapshot like any untracked read (D-23, #3946). */
function markVerdictReader(window) {
  const c = creatorPass(context);
  if (c && tracking) {
    // A pass `reask` returned to the frame answers for the hold's world (D-25).
    if (left.has(c)) return (watchVerdict(c, true), 1);
    // A pass created by one that read a held or staged value has no screen
    // and stays that world's (D-25, #4341). A first probing pass that
    // already joined a hold by a plain read restarts as a verdict reader,
    // so its reads agree in either order (D-25, #4777).
    const restart =
      c === adopter &&
      flushTransaction &&
      c._flags & REACTIVE_JOINED &&
      !(c._config & CONFIG_VERDICT) &&
      !(c._statusFlags & STATUS_UNINITIALIZED);
    c._flags |=
      REACTIVE_PROBED |
      REACTIVE_WINDOW |
      (window << WINDOW_KINDS) |
      (creatorReads(c, 0) || ((c._config |= CONFIG_VERDICT), 0));
    if (restart && c._config & CONFIG_VERDICT) {
      // Parks the pass with no visible pending; its tail re-runs it this flush.
      setAdopter(null);
      dropFlushTx();
      c._flags |= REACTIVE_MISSED_WAKE;
      c._loading = true;
      throw new NotReadyError(null);
    }
    // Observe tier: the windows entered (1 `isPending`, 2 `latest`).
    c._devWindows = (c._devWindows ?? 0) | window;
  }
}
function latest(fn) {
  if (markVerdictReader(2)) return fn();
  const prev = latestActive,
    prevVerdict = verdict;
  latestActive = true;
  setVerdict(verdictValue);
  try {
    return fn();
  } finally {
    latestActive = prev;
    setVerdict(prevVerdict);
  }
}
/**
 * Reports whether what `fn` reads is still waiting on an update. Read the
 * values in question inside `fn`: the probe answers `true` while any of
 * them depends on a change that is in flight — a write or an async source
 * that has started producing a new value whose result has not landed yet —
 * and `false` once the latest value is the one on screen. The answer is
 * tracked like any read, so a memo or effect that calls `isPending` re-runs
 * when it flips.
 *
 * A read that has no value yet is loading, not pending: inside `fn` it
 * follows the surrounding `Loading` path like any other read, and the probe
 * reports only updates to a value that already exists. A bare `refresh()`
 * does not flip `isPending`; pair it with `affects()` to make the re-ask
 * visible.
 *
 * @param fn expression to probe; read the values in question inside it
 *
 * @returns `true` while a read inside `fn` depends on a change that has not committed yet
 *
 * @example
 * ```ts
 * const [query, setQuery] = createSignal("");
 * const results = createMemo(() => searchProducts(query()));
 * const stale = createMemo(() => isPending(results));
 *
 * setQuery("shoes");
 * // stale() is true until the new results land, while results() keeps
 * // serving the previous list — dim it instead of showing a spinner.
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/reactivity/is-pending
 */
function isPending(fn) {
  const plain = markVerdictReader(1);
  const prevProbing = probing;
  const prevFound = probeFound;
  const prevVerdict = verdict;
  probing = !plain;
  probeFound = false;
  if (!plain) setVerdict(verdictValue);
  try {
    fn();
    return probeFound;
  } catch (e) {
    // A node with no answer suspends when owned and reads false unowned; a
    // real error is swallowed and the probe keeps what it found (D-23, S-51).
    if (e instanceof NotReadyError && !probeFound && context !== null) throw e;
    // Dev: the probe's own swallow is not a user catch (D-42).
    forgetThrownRead();
    return probeFound;
  } finally {
    probing = prevProbing;
    probeFound = prevFound;
    setVerdict(prevVerdict);
  }
}
/** A staged error throws (D-41). */
function staged(el) {
  if (el._config & CONFIG_STAGED_ERROR) throw el._pendingValue;
  return el._pendingValue;
}
/** `latest` of a held node: the flushed staging; an unflushed rewrite re-runs
 * its reader in the flush that carries it (F-10). */
function heldLatest(el, c) {
  const u = unflushedValue(el);
  if (u === NOT_PENDING) return staged(el);
  if (c) markLateLinker(c);
  return u;
}
/** The read inside a verdict window (D-22, D-23), off `read`'s hot path.
 * Links and pulls like a plain read, then answers by the node's state: a
 * lane's node, a held node, a staging this flush, a flight, or an error. */
function verdictValue(el, c) {
  const owner = el;
  const owned = !!c && !(c._config & CONFIG_CHILDREN_FORBIDDEN);
  const tracked = owned && tracking;
  // A pulled memo's pass is its own, outside the window (D-25, #4366, #4429).
  const prevProbing = probing;
  const prevLatest = latestActive;
  probing = false;
  latestActive = false;
  setVerdict(null);
  prepareComputed(owner);
  if (c && tracking) {
    link(el, c);
    // Record where this staging was asked, for `holdStaging` (#4544).
    if (el._pendingValue !== NOT_PENDING)
      ext(el)._askedIn = el._config & CONFIG_HELD ? txOf(el) : flushTransaction || clock;
    if (el._fn) pullComputed(owner, c);
  }
  setVerdict(verdictValue);
  latestActive = prevLatest;
  probing = prevProbing;
  // A hydration claim reads the server's frame, whole (F-1, #4570).
  const sv =
      snapshotCaptureActive && c && c._config & CONFIG_IN_SNAPSHOT_SCOPE
        ? el._x?._snapshotValue
        : undefined,
    served = sv !== undefined && !(owner._statusFlags & STATUS_ERROR);
  if (served) {
    const snapshot = sv === NO_SNAPSHOT ? undefined : sv;
    if ((el._pendingValue !== NOT_PENDING ? el._pendingValue : el._value) !== snapshot)
      c._flags |= REACTIVE_SNAPSHOT_STALE;
    return snapshot;
  }
  // An `affects()` mark is a declared new question (F-9).
  if (probing && GlobalQueue._marked?.(owner)) {
    probeFound = true;
    if (tracked && flushTransaction) verdictRead(c, flushTransaction);
  }
  if (
    el._config & CONFIG_OVERRIDE &&
    !(owner._statusFlags & STATUS_ERROR && !(el._config & CONFIG_GUESS))
  ) {
    const l = txOf(el),
      flying = (owner._statusFlags & STATUS_PENDING) !== 0 && !quietPending(owner),
      guess = el._config & CONFIG_GUESS,
      // NOT_PENDING: born in the lane with no answer yet, so loading (F-9, #4363).
      v = guess ? 0 : latestActive && !flying ? latestOf(el) : screenOf(el);
    if (v !== NOT_PENDING) {
      const inLane = passLane !== null && sameLane(passLane, l);
      if (tracked) verdictRead(c, l._parent ?? l);
      if (guess) {
        if (
          probing &&
          (flying ||
            (el._pendingValue !== NOT_PENDING &&
              (!el._equals || !el._equals(latestOf(el), el._pendingValue))))
        )
          probeFound = true;
        // A folded keyed arrangement reads its world whole; D-22's guess
        // clause is not built for it (D-22 status).
        if (latestActive && l.folded) {
          return l.folded(null, el);
        }
        return latestOf(el);
      }
      // A landed answer the lane has not shown is still pending (D-23,
      // S-54), as is a parked frame flight of its hold (#4365).
      if (probing && (flying || parkUp(resolveTx(l._parent ?? l)))) probeFound = true;
      else if (
        probing &&
        !inLane &&
        l._held &&
        !el._x._shown &&
        el._pendingValue !== NOT_PENDING &&
        (!el._equals || !el._equals(el._value, el._pendingValue))
      ) {
        probeFound = true;
        if (tracked) staleReader(c, l);
      }
      return v;
    }
  }
  // Dev: strict-read scopes warn, but a verdict read of a pending node does not throw.
  if (strictRead && strictReadActive())
    warnStrictReadUntracked(strictRead, {
      ownerId: c?.id,
      ownerName: c?._name,
      nodeName: owner?._name
    });
  const uninitialized = (owner._statusFlags & STATUS_UNINITIALIZED) !== 0;
  if (el._config & CONFIG_HELD) {
    if (!uninitialized) {
      const t = txOf(el);
      // A render effect run off a blocked verdict lane shows the committed
      // value and re-derives at the reveal (D-25, S-30).
      if (
        tracked &&
        !probing &&
        c._type === EFFECT_RENDER &&
        !passLane &&
        t._verdict?._held &&
        flushTransaction !== t
      ) {
        staleReader(c, t._verdict);
        return el._value;
      }
      // A pass that asks is display-ahead like any other (F-6, S-51).
      if (tracked) verdictRead(c, t);
      // A pass the hold holds is the hold's work: its probe is final at the
      // landing, never true beside the fresh value (D-23, #4648). An async
      // memo is not re-run, which would ask again (F-9).
      if (probing)
        if (c?._config & CONFIG_HELD && !c._x._transaction?._lane) {
          // In the landing flush the probe reads the landed world (F-1, #4662).
          if (el._pendingValue !== NOT_PENDING && !blocked(t)) {
            watchVerdict(c);
            return staged(el);
          }
          if (!c._x._q) staleReader(c, t);
        } else {
          if (heldNotFinal(owner)) probeFound = true;
          // Not held yet: re-run if the flush parks (D-23). Not an async or
          // first pass, which would ask again (F-9).
          if (owned && !tracked && !c._x?._q && !(c._statusFlags & STATUS_UNINITIALIZED))
            watchVerdict(c);
        }
      if (latestActive && el._pendingValue !== NOT_PENDING) {
        // A corrected shown guess stays `latest`'s value until the commit (D-34, #4689).
        if (keptGuess(el) && blocked(t)) {
          return el._value;
        }
        // An action body reading another hold's proposal entangles with it (D-7).
        if (!tracked && flushTransaction && !globalQueue._running) joinFuture(t);
        return heldLatest(el, tracked ? c : null);
      }
      if (latestActive) aheadError(owner);
      return el._value;
    }
    // Born held: no committed value, so no verdict (F-9); `latest` shows the staging (D-22).
    if (
      el._pendingValue !== NOT_PENDING &&
      (!(owner._statusFlags & STATUS_PENDING) || (latestActive && !probing))
    ) {
      if (probing && owned) joinPass(c, el);
      return staged(el);
    }
  } else if (el._pendingValue !== NOT_PENDING) {
    // A first answer re-asked in its own tick has a verdict (F-9, D-22);
    // an error is no value (D-41).
    if (
      owner._statusFlags & STATUS_PENDING &&
      (!uninitialized || (probing && !(el._config & CONFIG_STAGED_ERROR)))
    )
      pendingVerdict(tracked ? c : null, owner);
    // Writes that net to the committed value are no proposal: final (D-8, #4485).
    else if (tracked && globalQueue._running)
      flushTransaction &&
      !el._fn &&
      el._pendingValue === el._value &&
      Object.is(el._pendingValue, el._value)
        ? route(c, flushTransaction)
        : provisionalVerdict(c, !latestActive && !uninitialized);
    // An unflushed write is invisible (F-10); watch the flush that carries it (#3078).
    else if (tracked && probing) watchVerdict(c);
    if (latestActive || uninitialized || (owned && !tracked)) {
      // In a flush the staging is visible (F-10); outside one, re-run in the
      // carrying flush.
      if (globalQueue._running) {
        return staged(el);
      }
      if (c) markLateLinker(c);
    }
    return el._value;
  }
  if (owner._statusFlags & STATUS_PENDING) {
    if (!uninitialized) {
      pendingVerdict(tracked ? c : null, owner);
      if (latestActive) aheadError(owner);
      return el._value;
    }
    // As in `read`: an untracked read of a pending node still links.
    if (c && !tracking && !spectating && el !== c) link(el, c);
    throw owner._x?._error;
  }
  if (el._fn && owner._statusFlags & STATUS_ERROR) throw owner._x._error;
  return el._value;
}
/** An async memo with no window of its own (it read a verdict memo) is the
 * hold's work while its flight is up, in a lane, or re-asked: it drops the
 * verdict posture and waits like any derivation (D-25, F-9; #3884, #4408).
 * Called at a pass's head and when `source` goes pending. */
function holdWork(n, source) {
  // Truthy result: CONFIG_ASYNC stays set.
  if (
    (n._config & (CONFIG_ASYNC | CONFIG_OWN_PROBE)) === CONFIG_ASYNC &&
    (!(n._statusFlags & STATUS_PENDING) ||
      n._x._transaction?._lane ||
      source._flags & REACTIVE_REASK)
  )
    return (n._config &= ~CONFIG_VERDICT);
}
/** A derivation of a staged error carries it with the same frame, marked
 * rather than re-run, so the frame its content owns is kept (D-41, #4562).
 * `EFFECT_TRACKED << 23` spans CONFIG_FRAME and every effect type. */
function stageMark(node, el) {
  if (
    el._config & CONFIG_STAGED_ERROR &&
    !(node._config & (CONFIG_REDERIVE | CONFIG_STAGED_ERROR | (EFFECT_TRACKED << 23)))
  ) {
    // Only a direct reader of the staged error carries it (#4640). A lane
    // reader does not: the staged truth is the hold's, shown at its reveal
    // (F-1, F-11, D-34; #4751, #4841).
    for (let d = node._deps; d; d = d._nextDep)
      if (
        d._dep._config & CONFIG_STAGED_ERROR &&
        !(
          node._x?._transaction?._lane &&
          (!(d._dep._config & CONFIG_GUESS) || d._dep._x._transaction === node._x._transaction)
        )
      ) {
        node._pendingValue = el._pendingValue;
        node._config |= CONFIG_STAGED_ERROR;
        queuePendingNode(node);
        // A derived store's readers re-derive from the staging (#4189).
        node._fam?.st(STATUS_ERROR);
        return settlePendingSource(node);
      }
  }
}
/** `el` was republished mainline with the value it had staged: a held reader
 * whose only held read was `el` keeps its pass and publishes as is (#4417). */
function keepReaders(el) {
  for (let s = el._subs; s !== null; s = s._nextSub) {
    const r = s._sub;
    if ((r._config & (CONFIG_HELD | CONFIG_STAGED)) !== (CONFIG_HELD | CONFIG_STAGED) || r._type)
      continue;
    let d = r._deps;
    while (d !== null && (d._dep === el || !(d._dep._config & CONFIG_HELD))) d = d._nextDep;
    if (d !== null) continue;
    // A mount a probing pass opened attaches at the landing, not now (D-25, S-46).
    if (heldMountUnder(r, null)) continue;
    deleteFromHeap(r, dirtyQueue);
    r._flags &= -4;
    r._config &= ~CONFIG_HELD;
    queuePendingNode(r);
    insertSubs(r);
  }
}
/** A run the landing holds (lanes.ts `waiting`) runs next round with its held runs. */
function attachWaits(node) {
  if (waiting?.has(node))
    return (
      globalQueue.enqueue(
        node._type,
        (node._boundRunEffect ??= GlobalQueue._runEffect.bind(null, node))
      ),
      true
    );
  return false;
}
/** Whether a plain read of `el` is served the hold's staging. A pass that
 * opened no window of its own inherits no posture (D-25, #4544): it is the
 * hold's work where it reads the hold's own sync write, unless a window
 * asked of that staging under the same hold (`_askedIn`, D-25 pair
 * atomicity). A frame reads the screen. No `el`: a plain store key with no
 * node, decided by posture alone, as for a signal. */
function holdStaging(c, el) {
  const r = c._x?._readIn,
    a = el?._x?._askedIn,
    h = el && resolveTx(el._config & CONFIG_HELD ? el._x._transaction : flushTransaction);
  // A sync verdict memo's value staged this flush is read plainly, as the
  // window inline would be (D-25, #4778).
  if (
    el &&
    !c._type &&
    !passLane &&
    el._fn &&
    (el._config & (CONFIG_VERDICT | CONFIG_ASYNC | CONFIG_HELD | CONFIG_OVERRIDE)) ===
      CONFIG_VERDICT
  )
    return true;
  return (
    r &&
    !(
      c._config & (CONFIG_OWN_PROBE | CONFIG_FRAME) ||
      c._flags & (REACTIVE_WINDOW | REACTIVE_PROBE_UNANSWERED)
    ) &&
    (!el ||
      (el._pendingValue !== NOT_PENDING &&
        !(el._config & (CONFIG_ASYNC | CONFIG_OVERRIDE)) &&
        a !== clock &&
        !(typeof a == "object" && resolveTx(a) === h) &&
        resolveTx(r) === h)) &&
    (!passLane || (passLane._parent._verdict === passLane && (setPassLane(null), true)))
  );
}
/** Parked frame flights. When the seam re-derives a verdict reader in its
 * lane, the frame pass's flight already asked the hold's question; the
 * landing adopts it instead of asking again (F-9, D-25; #4365, #4393).
 * Adopted at the reader's own turn in the next round, not at the landing,
 * so the hold is judged with the flight on it (F-1). Through a promise of
 * its own, so the frame pass's listener cannot land it twice. */
const armed = [];
/** Readers whose parked frame flight is up: the hold waits on it, however
 * the lane pass came out (F-2, #3884). */
const parks = [];
function park(r, t) {
  const p = (r._x._parked = r._x._inFlight),
    done = () => {
      const i = parks.indexOf(r);
      if (i !== -1 && r._x._parked === p) (parks.splice(i, 1), schedule());
    };
  // Only a flight a reader on screen waits on, judged as `blockedBy` does
  // and before the lane's passes replace the frame's pending (F-3, #4504).
  if (
    t._nodes.some(n => {
      if (n._statusFlags & STATUS_PENDING && (n === r || n._x._pendingSources?.has(r))) {
        if (n._type === EFFECT_RENDER) return true;
        for (let s = n._subs; s; s = s._nextSub) {
          const e = s._sub;
          if (
            e._type === EFFECT_RENDER
              ? e._statusFlags & STATUS_PENDING || e._flags & REACTIVE_FRAME_READ
              : e._config & CONFIG_REDERIVE && e._statusFlags === STATUS_PENDING
          )
            return true;
        }
      }
    })
  ) {
    parks.indexOf(r) === -1 && parks.push(r);
    p.then(done, done);
  }
}
function parkUp(t) {
  for (let i = parks.length - 1; i >= 0; i--) {
    const x = parks[i]._x,
      l = x._transaction;
    if (!x._parked || parks[i]._flags & REACTIVE_DISPOSED) parks.splice(i, 1);
    else if (l?._lane && resolveTx(l._parent) === t) return true;
  }
  return false;
}
/** Passes `reask` returned to the frame; truthy when the lane pass was unanswered. */
const left = new Map();
/** A write the hold took reached lane member `c` while the hold waits on a
 * parked flight of it: that flight answers a superseded question, so the
 * new one is asked now by returning `c` (and the lane members it reads) to
 * the frame (F-2, F-9, D-25; #3884). */
function reask(c, l, cur) {
  if (left.has(c)) return;
  const i = parks.indexOf(c);
  if (i === -1) {
    if (!c._type && c._x?._transaction === l)
      for (let s = c._subs; s; s = s._nextSub) reask(s._sub, l, cur);
    return;
  }
  parks.splice(i, 1);
  leave(c, l, cur);
}
/** Seat a lane pass from its first read. */
function seat(r, t) {
  list(r, verdictLane(t));
  r._flags |= REACTIVE_LANE_DIRTY;
}
function leave(c, l, cur) {
  const x = c._x;
  left.set(c, !x._shown && c._statusFlags & STATUS_PENDING);
  covers.delete(c);
  x._shown = false;
  x._transaction = null;
  x._parked = undefined;
  c._pendingValue = NOT_PENDING;
  c._config &= -34817;
  if (c !== cur) ((c._flags |= REACTIVE_DIRTY), enqueueSub(c));
  for (let d = c._deps; d; d = d._nextDep) {
    const n = d._dep;
    if (n._fn && n._x?._transaction === l && !left.has(n)) leave(n, l, cur);
  }
}
const adoptParked = (r, visit) => {
  if (visit === 2) {
    const l = r._x._transaction;
    if (parks.length && isVerdict(l)) reask(r, l, r);
    return false;
  }
  if (visit) {
    const i = armed.indexOf(r);
    if (i < 0) return false;
    armed.splice(i, 1);
    const parked = r._x._parked;
    deleteFromHeap(r, dirtyQueue);
    r._flags &= -4;
    r._x._parked = undefined;
    releaseFlightTeardown(r);
    try {
      handleAsync(r, parked.then());
    } catch {}
    r._x._blocked = true;
    notifyStatus(r, STATUS_PENDING, new NotReadyError(r));
    return true;
  }
  // A sync memo re-derived at a landing voids its readers' held runs: they
  // re-run in the landing's frame (F-1).
  if (!(r._type || r._config & CONFIG_ASYNC))
    for (let s = r._subs; s !== null; s = s._nextSub) {
      const e = s._sub;
      if (e._type && e._modified) {
        e._modified = false;
        enqueueSub(e);
      }
      arm$1(e);
    }
  arm$1(r);
  return false;
};
const arm$1 = r =>
  r._x?._parked &&
  !(r._flags & (REACTIVE_DIRTY | REACTIVE_CHECK | REACTIVE_IN_HEAP)) &&
  !armed.includes(r) &&
  armed.push(r);
/** The seam's park step (lanes.ts `laneSeams`, `lanesBlocked`). `r` set:
 * its probe is unanswered, truthy when nothing re-derives. `r` null: does
 * hold `t` wait on a parked flight. Both null: the seam's end, clear. */
function r5(r, t) {
  if (!r) {
    if (t) return parkUp(t);
    armed.length = 0;
    left.clear();
    return;
  }
  if (isThenable(r._x?._inFlight)) {
    if (!blocked(t)) return 1;
    park(r, t);
    // Seated from its first read, so a plain read before its window does not ask again (D-25).
    seat(r, t);
  } else if (blocked(t)) {
    if (left.has(r)) seat(r, t);
    // Async memos over a sync reader: their frame flights are adopted too (F-9).
    for (let s = r._subs, n; s; s = s._nextSub)
      if (
        isThenable((n = s._sub)._x?._inFlight) &&
        n._x._pendingSources?.has(n) &&
        n._config & CONFIG_HELD &&
        txOf(n) === t &&
        // A flight asked over the screen is no frame question.
        !(n._flags & REACTIVE_FRAME_READ)
      ) {
        park(n, t);
        // An unanswered lane pass `reask` took asks again; nothing else would.
        if (left.get(n)) (seat(n, t), enqueueSub(n));
      }
  }
}
/** At the flush: writes that net to the tick's start made no change (D-8),
 * so readers with every source unchanged leave the heap and ask nothing
 * (F-9). Verdict readers still re-run. Done here rather than per write: an
 * extra arm in the setter cost it its inlining. */
function revert() {
  for (const n of pendingNodes)
    // Netting writes are staged as the committed value itself, so only those
    // reach the comparator. A derivation's staging is an answer, not a write.
    if (n._pendingValue === n._value && (!n._fn || n._flags & REACTIVE_MANUAL_WRITE)) unwake(n);
  // Held slots rewritten since the last flush are not in `pendingNodes` (F-10).
  if (heldRewrites.length !== 0) for (const n of heldRewrites.splice(0)) unwake(n);
}
function unwake(n) {
  if (n._pendingValue === NOT_PENDING || !isSame(n, tickBase(n), n._pendingValue)) return;
  for (let s = n._subs; s !== null; s = s._nextSub) {
    const sub = s._sub;
    if (
      !(sub._flags & REACTIVE_IN_HEAP) ||
      sub._flags & REACTIVE_MISSED_WAKE ||
      sub._config & CONFIG_VERDICT
    )
      continue;
    let same = true;
    for (let d = sub._deps; d !== null && same; d = d._nextDep) {
      const dep = d._dep;
      const fn = dep._fn !== undefined;
      // A user write on a derivation counts as a written source (#4991).
      same =
        !(fn && dep._flags & (REACTIVE_IN_HEAP | REACTIVE_DIRTY | REACTIVE_CHECK)) &&
        (dep._pendingValue === NOT_PENDING ||
          (fn && !(dep._flags & REACTIVE_MANUAL_WRITE)) ||
          isSame(dep, tickBase(dep), dep._pendingValue));
    }
    if (same) {
      deleteFromHeap(sub, dirtyQueue);
      sub._flags &= -4;
    }
  }
}
/** A pass whose every source is at its committed value is a quiet re-ask
 * (F-9). Not under a guess or in a lane, nor with no committed value. */
function pass(el, tail) {
  if (!tail) {
    // Not on the extension's literal: writing it on every `_x` would change its shape.
    el._x._aheadError =
      (el._config & (CONFIG_STAGED_ERROR | CONFIG_HELD)) === (CONFIG_STAGED_ERROR | CONFIG_HELD)
        ? el._pendingValue
        : undefined;
    return false;
  }
  if (guessing || passLane !== null || el._statusFlags & STATUS_UNINITIALIZED) return false;
  for (let d = el._deps; d !== null; d = d._nextDep) {
    const dep = d._dep;
    if (
      dep._pendingValue !== NOT_PENDING
        ? !isSame(dep, dep._value, dep._pendingValue)
        : dep._fn && dep._statusFlags & STATUS_PENDING
    )
      return false;
    if (d === el._depsTail) break;
  }
  return true;
}
installVerdictHooks(
  observeFlight,
  holdWork,
  stageMark,
  keepReaders,
  attachWaits,
  holdStaging,
  adoptParked,
  r5,
  revert,
  pass
);
/** An `affects()` mark on a store record with no node (store/affects.ts). */
GlobalQueue._witnessMark = () => {
  if (probing) probeFound = true;
};

const ACTION_CALLED_IN_OWNED_SCOPE_MESSAGE =
  "[ACTION_CALLED_IN_OWNED_SCOPE] Calling an action inside an owned scope (component, computation) is not allowed. " +
  "Call it from an event handler or another imperative scope.";
/** A slice of the body runs in the action's transaction: the flush that
 * carries its writes parks into it (`joinFuture` — merging whatever the tick
 * already joined: O1, an action that opens in a tick owns it; a second
 * action in the tick, or one called from the body, merges into the first).
 * A resumed slice drains at once: the seam parks what it wrote (the action
 * holds the transaction open) or, the body having returned, lands it. A
 * nested action resuming synchronously inside the outer body does not drain
 * — that would park the outer's remaining writes' frame mid-slice; the outer
 * step's own return does. */
function inTransaction(t, q, fn) {
  // The slice and its flush ask this action's question (F-11):
  // the guesses it writes and the flights it starts are stamped with it.
  const prevQ = question;
  setQuestion(q);
  joinFuture(t);
  const result = fn();
  if (actionDepth === 0) flush();
  setQuestion(prevQ);
  return result;
}
/**
 * The primitive for mutations: imperative async workflows whose *writes span
 * an async gap* — optimistic write, server round-trip, reconciling write —
 * where intermediate state must not leak and failure must revert cleanly
 * (pair with `createOptimistic` / `createOptimisticStore`).
 *
 * Navigation-shaped updates do not need an action. A plain setter call is
 * enough: reads pull the async, and downstream async computeds hold their
 * previous values per-node until the new ones are ready (`isPending` /
 * `latest` expose the in-flight state). Reach for `action` only when writes
 * happen *after* async work, not merely upstream of it.
 *
 * Framework-level actions (router form actions, server actions) are
 * specializations of this primitive: they are actions in exactly this sense —
 * the same transactional semantics — with form binding, serialization, and
 * submission tracking layered on top. The shared name is deliberate.
 *
 * Wraps a generator function so each invocation runs as a single transaction
 * that holds every signal/store write between yields. The surrounding UI sees
 * one atomic update when the action completes; nothing is committed until
 * every action in the transaction has returned.
 *
 * `yield` is the transaction-safe suspension point: the action waits for a
 * yielded promise and re-enters the transaction before running the code after
 * it. A plain `await` does NOT — the runtime has no hook into an async
 * generator's internal await continuations, so code between an `await` and
 * the next `yield` runs OUTSIDE the transaction: writes to fresh signals
 * commit immediately, and anything that creates a reader there — `until()`,
 * `latest()`, a memo or effect, a mount — is created mainline, where a read of
 * this action's held state makes it born held (F-7): staged with the
 * transaction and replayed at its commit. `await` is still the ergonomic
 * choice for typed results; just put a bare `yield` before any write or
 * reader creation that follows it — including the expression of the next
 * `yield`, which is evaluated before the step re-enters:
 *
 * ```ts
 * const saved = await api.createTodo(text); // typed result
 * yield; // re-enter the transaction before writing or reading
 * setTodos(t => { ... });
 * yield until(() => todos.some(t => t.id === saved.id));
 * ```
 *
 * The same holds inside an async helper the action yields
 * (`yield (async () => { ... })()`): everything after its first `await` runs
 * outside the transaction. An `await refresh(x)` or `await until(...)` there
 * waits for this action's commit, which waits on the helper, so it hangs
 * whenever `x` re-ran during the action (#4263) and works otherwise. Call
 * `refresh(x)` in the generator once the helper returns; `yield` it only when
 * the action needs the fresh value.
 *
 * (For the same reason, don't call `flush()` inside an action body — it
 * cannot reveal the held writes and is refused.)
 *
 * Each call returns a `Promise` that resolves with the generator's return
 * value, or rejects if it throws. Pair with `createOptimistic` /
 * `createOptimisticStore` to apply tentative writes that auto-revert if the
 * action fails.
 *
 * @example
 * ```ts
 * const [todos, setTodos] = createOptimisticStore<Todo[]>([]);
 *
 * const addTodo = action(async function* (text: string) {
 *   const tempId = crypto.randomUUID();
 *   setTodos(t => { t.push({ id: tempId, text, pending: true }); }); // optimistic
 *   const saved = await api.createTodo(text); // network round-trip, typed
 *   yield; // re-enter the transaction
 *   setTodos(t => {
 *     const i = t.findIndex(x => x.id === tempId);
 *     if (i >= 0) t[i] = saved;
 *   });
 *   return saved;
 * });
 *
 * await addTodo("buy milk");
 * ```
 */
function action(genFn) {
  return (...args) => {
    // Invoking an action opens a transaction — like a write, it is invalid
    // synchronously inside an owned scope. The write guard can't catch this
    // at the real hazard point: post-await writes run with no ambient owner,
    // and a computation tracking what its action writes livelocks (every
    // write retriggers the compute, which fires a fresh invocation). Same
    // scope test as setSignal: leaf imperative scopes (tracked effects,
    // onSettled) stay legal.
    {
      const owner = getOwner();
      if (owner && !(owner._config & CONFIG_CHILDREN_FORBIDDEN) && !inRefCallback()) {
        emitDiagnostic({
          code: "ACTION_CALLED_IN_OWNED_SCOPE",
          kind: "write",
          severity: "error",
          message: ACTION_CALLED_IN_OWNED_SCOPE_MESSAGE,
          ownerId: owner.id,
          ownerName: owner._name
        });
        throw new Error(ACTION_CALLED_IN_OWNED_SCOPE_MESSAGE);
      }
    }
    return new Promise((resolve, reject) => {
      const it = genFn(...args);
      // The action's transaction, held open until the body returns. The
      // first slice's writes are carried by the tick's scheduled flush.
      const t = newTransaction(false);
      t._open = 1;
      t._acted = true;
      const q = nextQuestion();
      const done = (v, e, failed = false) => {
        resolveTx(t)._open--;
        schedule();
        failed ? reject(e) : resolve(v);
      };
      const step = (v, err) => {
        let r;
        // Attribution hooks bracket the synchronous slice of generator body
        // this step runs (up to the next yield): writes inside are the
        // action's. Both sites sit outside the try (attribution-hooks.ts).
        if (attrHooks !== null) attrHooks.actionStepStart(it, genFn.name || undefined);
        // The body is on the stack between these brackets: flush() is
        // refused inside (FLUSH_IN_ACTION, scheduler.ts).
        enterAction();
        // Dev: the body's reads are imperative, not post-await reads of the
        // continuation that invoked the action (UNTRACKED_READ_AFTER_AWAIT);
        // and it is an imperative scope of its own, not the effect callback
        // it may have been called from — a root created in it (`until`,
        // `resolve`) is deliberate (PRIMITIVE_IN_EFFECT_CALLBACK is off).
        let effectCallback = false;
        let effectEvent;
        let effectOwner = null;
        {
          enterCallback();
          effectCallback = inEffectCallback;
          effectEvent = effectCallbackEvent;
          effectOwner = effectCallbackOwner;
          setEffectCallback(false);
        }
        try {
          r = err ? it.throw(v) : it.next(v);
        } catch (e) {
          exitAction();
          (exitCallback(), setEffectCallback(effectCallback, effectEvent, effectOwner));
          if (attrHooks !== null) attrHooks.actionStepEnd(it);
          return done(undefined, e, true);
        }
        exitAction();
        (exitCallback(), setEffectCallback(effectCallback, effectEvent, effectOwner));
        if (attrHooks !== null) attrHooks.actionStepEnd(it);
        // A rejected iterator result (async generators) means the error already
        // escaped the generator body — it is completed, and throwing back in
        // would just reject again forever. Settle instead.
        if (isThenable(r)) return void r.then(run, e => done(undefined, e, true));
        run(r);
      };
      const run = r => {
        if (r.done) return done(r.value);
        // Thenable assimilation can itself throw synchronously (a `then`
        // getter, or a `then()` method that throws — #2918). Match `await`
        // semantics: the failure is thrown back into the generator at the
        // yield point (catchable there); if uncaught, step()'s guard settles
        // the action. The settled flag implements A+ 2.3.3.3.4.1: a throw
        // after the thenable already called a callback is ignored.
        let settled = false;
        try {
          if (isThenable(r.value))
            return void r.value.then(
              v => {
                if (settled) return;
                settled = true;
                inTransaction(t, q, () => step(v));
              },
              e => {
                if (settled) return;
                settled = true;
                inTransaction(t, q, () => step(e, true));
              }
            );
        } catch (e) {
          if (settled) return;
          settled = true;
          return void inTransaction(t, q, () => step(e, true));
        }
        inTransaction(t, q, () => step(r.value));
      };
      // The first slice's writes are carried by the tick's scheduled flush;
      // the slice itself asks the action's question (its guesses are stamped
      // at the write).
      joinFuture(t);
      const prevQ = question;
      setQuestion(q);
      step();
      setQuestion(prevQ);
    });
  };
}

let externalSourceConfig = null;
/**
 * Registers a factory that bridges external reactive systems (e.g. MobX, Vue refs)
 * into Solid's tracking graph. Every computation will be wrapped so that the
 * external library can track its own dependencies alongside Solid's.
 *
 * Multiple calls pipe together: each new factory wraps the previous one.
 *
 * @param config.factory receives `(fn, trigger)` — wrap fn execution in external tracking,
 *   call trigger when external deps change. Return `{ track, dispose }`.
 * @param config.untrack optional wrapper for `untrack` — disables external tracking too.
 *
 * @example
 * ```ts
 * // Bridge an external "subscribe / notify" library into Solid's graph.
 * // `factory` wraps every Solid compute so the external library can attach
 * // its own dependency tracker; `trigger` re-runs the compute on external
 * // change. `untrack` mirrors Solid's `untrack()` into the external library
 * // so that reads inside `untrack(...)` don't get tracked twice.
 * enableExternalSource({
 *   factory: (compute, trigger) => {
 *     const sub = externalLib.subscribe(trigger);
 *     return {
 *       track: prev => externalLib.run(() => compute(prev)),
 *       dispose: () => sub.unsubscribe()
 *     };
 *   },
 *   untrack: fn => externalLib.untracked(fn)
 * });
 * ```
 */
// Wires a freshly created computed through the active external-source bridge.
// Lives here (installed as a scheduler hook while a config is active) rather than
// inline in core: esbuild cannot literal-track the mutable config binding the
// way rollup does, so an inline `if (externalSourceConfig)` block ships in
// every bundle even though only enableExternalSource() can make it reachable.
function wireExternalSource(self) {
  const bridgeSignal = signal(undefined, { equals: false, ownedWrite: true });
  const source = externalSourceConfig.factory(self._fn, () => {
    setSignal(bridgeSignal, undefined);
  });
  cleanup(() => source.dispose());
  self._fn = prev => {
    read(bridgeSignal);
    return source.track(prev);
  };
}
function externalUntrack(fn) {
  return externalSourceConfig.untrack(fn);
}
// The hooks mirror the config's liveness exactly (installed on enable,
// removed on reset) so core's presence checks stay equivalent to the old
// `externalSourceConfig` truthiness checks.
function syncExternalHooks() {
  if (externalSourceConfig) setExternalHooks(wireExternalSource, externalUntrack);
  else setExternalHooks(undefined, undefined);
}
function enableExternalSource(config) {
  const { factory, untrack: untrackFn = fn => fn() } = config;
  if (externalSourceConfig) {
    const { factory: oldFactory, untrack: oldUntrack } = externalSourceConfig;
    externalSourceConfig = {
      factory: (fn, trigger) => {
        const oldSource = oldFactory(fn, trigger);
        const source = factory(x => oldSource.track(x), trigger);
        return {
          track: x => source.track(x),
          dispose() {
            source.dispose();
            oldSource.dispose();
          }
        };
      },
      untrack: fn => oldUntrack(() => untrackFn(fn))
    };
  } else {
    externalSourceConfig = { factory, untrack: untrackFn };
  }
  syncExternalHooks();
}

// Deferred hydration, dependency edges: plain-signal identities.
// While on (`setSignalIdentities`; the server-components client turns it on
// before hydrating), a plain signal created under an id-bearing owner is
// named `<owner id>~<n>`, n counting the plain signals (and plain store
// roots: `createStore` over an object, store/edges.ts) created under that
// owner in its current pass (transparent owners defer to the nearest
// id-bearing one, as child ids do). The server names the same signal the
// same way and restarts the count on every re-run, so an edge it shipped (a
// slot fill read it) matches the client node; the write hook receives the
// node and looks its name up (`signalName`). The count restarts when the owner's pass
// generation (`_depGen`, bumped by every recompute) moves on. Only the
// frames client imports this module, so nothing here ships without it.
// Names live in a side table, not on the node: frames reads them across the
// package boundary, where a mangled field would not survive.
let counts = null;
const names = new WeakMap();
/** @internal The identity of a plain signal named while identities were on. */
function signalName(node) {
  return node === undefined ? undefined : names.get(node);
}
function identify(node) {
  let o = getOwner();
  if (!o) return;
  while (o._config & CONFIG_TRANSPARENT && o._parent) o = o._parent;
  if (o.id == null) return;
  const gen = o._depGen;
  let c = counts.get(o);
  if (c === undefined || c[0] !== gen) counts.set(o, (c = [gen, 0]));
  const sid = o.id + "~" + c[1]++;
  // A store root named twice (one object made into two stores, or a store
  // handed to `createStore`) is one target under two server names: its
  // writes cannot be attributed (an empty name).
  names.set(node, names.has(node) ? "" : sid);
}
/** @internal Name plain signals from now on (`on`), or stop. */
function setSignalIdentities(on) {
  counts = on ? counts || new WeakMap() : null;
  signalHook = on ? identify : null;
}
/**
 * @internal Called with every plain signal `createSignal` makes, and every
 * plain store root `createStore` makes, while set (the namer above). Never
 * set unless `setSignalIdentities` is imported, so a bundle without it keeps
 * only a null check.
 */
let signalHook = null;
/**
 * @internal Dev only: `watch` is called with every node read while set (the
 * dev hook `onRead`) — the frames client's identity-drift check watches the
 * named signals a deferred occurrence reads as it mounts. A no-op in prod.
 */
function setReadWatch(watch) {
  DEV$1.hooks.onRead = watch || undefined;
}

/**
 * Context provides a form of dependency injection. It is used to save from needing to pass
 * data as props through intermediate components. This function creates a new context object
 * that can be used with `getContext` and `setContext`.
 *
 * A default value can be provided here which will be used when a specific value is not provided
 * via a `setContext` call.
 */
function createContext(defaultValue, description) {
  // Dev: an explicit `undefined` default is still no default (`undefined`
  // means "not provided"); remembered so a failed read can say so (#4181).
  if (arguments.length && defaultValue === undefined)
    return { id: Symbol(description), defaultValue, _undefinedDefault: true };
  return { id: Symbol(description), defaultValue };
}
/**
 * Low-level owner-targeted context read. The user-facing read API is
 * `useContext` (in `solid-js`), which wraps this primitive. Exposed here for
 * cross-package wiring (e.g. hydration-aware context plumbing).
 *
 * @throws `NoOwnerError` in dev if there's no owner at the time of call; production
 *   reads the default instead (see below).
 * @throws `ContextNotFoundError` if a context value has not been set yet.
 *
 * @internal
 */
function getContext(context, owner = getOwner()) {
  // `undefined` alone means unset — a provided `null` is a value (no `??`).
  let value;
  if (owner) value = owner._context[context.id];
  // Dev keeps the loud error. Production reads with no owner as if under an
  // empty root: the default, else `ContextNotFoundError`. That is what dev
  // gives a component created outside any owner, since dev `createComponent`
  // wraps every component in a root; prod `createComponent` does not.
  else throw new NoOwnerError();
  if (value === undefined) value = context.defaultValue;
  if (value === undefined) {
    // Dev names the context and why it has no value: no provider, or a
    // provider that passed `undefined` (the key is set, to `undefined`).
    throw new ContextNotFoundError(context, context.id in owner._context);
  }
  return value;
}
/**
 * Low-level owner-targeted context write. The user-facing API is
 * `createContext` (in `solid-js`); its provider component wraps this
 * primitive. Exposed here for cross-package wiring.
 *
 * @throws `NoOwnerError` if there's no owner at the time of call.
 *
 * @internal
 */
function setContext(context, value, owner = getOwner()) {
  if (!owner) {
    throw new NoOwnerError();
  }
  // We're creating a new object to avoid child context values being exposed to parent owners. If
  // we don't do this, everything will be a singleton and all hell will break lose.
  owner._context = {
    ...owner._context,
    [context.id]: value === undefined ? context.defaultValue : value
  };
}

/**
 * Effects are the leaf nodes of our reactive graph. When their sources change, they are
 * automatically added to the queue of effects to re-execute, which will cause them to fetch their
 * sources and recompute
 */
function effect(compute, effect, error, options) {
  const isUser = !!options?.user;
  const node = createEffectNode(
    compute,
    effect,
    error,
    isUser ? EFFECT_USER : EFFECT_RENDER,
    options
  );
  recompute(node, true);
  // A first pass that read a staged value was staged itself (recompute: born
  // staged); the flush's commit replays this effect. Its first run is not this
  // creation's (F-7). A lane's node's run is the lane's: queued, not run
  // now — into its own pass's lane (its transaction: the creator's lane, or
  // the one a lane read moved the pass into, F-6/F-7) — and released at the
  // reveal (lanes.ts). Not a lane's node: the creator's (`enqueue`'s default
  // for no transaction; the frame for a transaction's).
  // `defer` skips the first run, whoever owes it. A node born staged
  // (#4543): its staged first value applies now, so the commit replays
  // nothing and the next pass gets it as `prev`, as on a plain mount (#4556;
  // D-12). A lane's node (#4557): it queues none.
  options?.defer
    ? applyStaged(node)
    : node._pendingValue === NOT_PENDING &&
      (node._x?._transaction?._lane || node._type === EFFECT_USER || options?.schedule
        ? globalQueue.enqueue(node._type, runEffect.bind(null, node), node._x?._transaction)
        : // Type 0: the synchronous first render, not a queued run (D-19).
          runEffect(node, 0));
  if (!node._parent) {
    const message =
      "[NO_OWNER_EFFECT] Effects created outside a reactive context will never be disposed";
    reportDiagnostic(
      emitDiagnostic(
        {
          code: "NO_OWNER_EFFECT",
          kind: "lifecycle",
          severity: "warn",
          message,
          ownerId: node.id,
          ownerName: node._name,
          data: { effectType: "effect" }
        },
        node
      )
    );
  }
}
function notifyEffectStatus() {
  const actualStatus = this._statusFlags;
  const actualError = this._x?._error;
  if (actualStatus & STATUS_ERROR) {
    globalQueue.notify(this, STATUS_PENDING, 0);
    if (this._type === EFFECT_USER) {
      // The error handler is the error arm of the effect phase (#2840 ruling):
      // queue it like the effect function. It runs in the same imperative,
      // writable scope and throws escalate the same way. No payload is queued — the node already carries `_statusFlags`/`_error`,
      // and the runner dispatches on them, so a recovery before the effect
      // phase takes the success arm instead. (Re-read: the boundary
      // notify above can run a redraw.)
      if (this._statusFlags & STATUS_ERROR) queueEffectRun(this);
      return;
    }
    // Dev locates the halt at the node that threw: the StatusError's
    // source, upstream of this effect when a computed failed. The rethrow is
    // the user's error, not the wrapper: the wrapper's `source` is a node, and
    // a reporter that serializes the throw walks the reactive graph.
    escalate(
      this,
      unwrapStatusError(actualError),
      actualError instanceof StatusError ? actualError.source : undefined
    );
  } else if (
    // A user effect's run owed for a pass a new question has since made
    // pending is for a world that will not commit (F-1, F-2): void — the
    // effect re-runs when that question settles (`settlePendingSource`). A
    // quiet re-ask keeps it: the pass still answers the question asked (F-9).
    this._type === EFFECT_USER
  ) {
    if (actualStatus & STATUS_PENDING && this._modified && !actualError?.source?._x?._reask)
      ext(this)._blocked = !(this._modified = false);
  } else if (this._type === EFFECT_RENDER) {
    globalQueue.notify(this, STATUS_PENDING | STATUS_ERROR, actualStatus, actualError);
    if (_hitUnhandledAsync && resetUnhandledAsync()) {
      // Async without a `Loading` ancestor is legal (the mount defers), so this
      // is a consistent FYI — an `Errored` above must not swallow it. The old
      // STATUS_ERROR re-notify here dated from when enforcement routed the
      // pending to the error boundary; that both suppressed the warning and
      // showed the error fallback in dev only (#2822). Reported once per
      // mount (resetUnhandledAsync gates), located at the first pending
      // effect's owner path.
      const message =
        "[ASYNC_OUTSIDE_LOADING_BOUNDARY] An async value was read outside a Loading boundary. The root mount will be deferred until all pending async settles.";
      reportDiagnostic(
        emitDiagnostic(
          {
            code: "ASYNC_OUTSIDE_LOADING_BOUNDARY",
            kind: "async",
            severity: "warn",
            message,
            ownerId: this.id,
            ownerName: this._name
          },
          this
        )
      );
    }
  }
}
/** An effect's error no boundary takes halts the system (boundary-or-halt):
 * halted with `cause` and rethrown; dev locates it at `subject` (default:
 * the effect). */
function escalate(node, cause, subject) {
  if (!globalQueue.notify(node, STATUS_ERROR, STATUS_ERROR)) {
    haltReactivityAt(cause, subject ?? node);
    throw cause;
  }
}
function runEffect(node, type) {
  if (!node._modified || node._flags & REACTIVE_DISPOSED) return;
  // A queued run behind a fallback (boundaries.ts) waits for the reveal: a
  // user effect's, which would read a DOM that is not attached, and a render
  // effect's update too (maintainer, 2026-10-02: "we held the queue but let
  // the sync renders through… we definitely shouldn't be showing portals
  // early") — a Portal's or head-tag's render effect writes outside the
  // hidden subtree. The synchronous first render on creation (`type` 0) is
  // not a queued run and goes through, whichever boundary above is on its
  // fallback then — an outer one about to reveal in this flush included
  // (#4343). `_modified` stays set; the boundary re-queues the run
  // (`release`).
  if (type && heldRunHook?.(node)) return;
  // An attach whose subtree holds a hole born into an open hold waits for
  // that hold (#4574, verdict.ts `attachWaits`).
  if (type && attachHook?.(node)) return;
  // Error arm (#2840), user effects only: a compute-phase error that is still
  // the node's settled state at effect time runs the bundle's error handler in
  // this same imperative, writable scope. Unwrap the StatusError used for
  // source tracking — user code gets the error it threw, as boundaries do. No
  // handler: log and keep the system alive (the run was skipped). A handler
  // (or logging) consumes the error; a handler throw falls to the shared
  // catch below and escalates boundary-or-halt like any effect-phase throw.
  // Render effects bypass: their errors route to boundaries synchronously in
  // notifyEffectStatus, and a runner queued by an earlier valueChanged in the
  // same flush must not be hijacked by a later-arriving error status.
  if (node._statusFlags & STATUS_ERROR && node._type === EFFECT_USER) {
    const err = unwrapStatusError(node._x?._error);
    node._prevValue = node._value;
    node._modified = false;
    try {
      node._errorFn
        ? node._errorFn(err, () => {
            const prevCleanup = node._cleanup;
            node._cleanup = undefined;
            prevCleanup?.();
          })
        : console.error(err);
    } catch (error) {
      // (D-42, dev: escalated, not caught — and the running pass, if any, is
      // the effect's creator, not a catcher.)
      forgetThrownRead();
      escalate(node, error);
    }
    return;
  }
  // Captured before the callback: its own throw errors the node below, but
  // the compute pass that produced `_value` was clean, so its tail still goes.
  const cleanPass = node._x?._error == null;
  let prevStrictRead = false;
  let prevStrictReadEvent;
  {
    prevStrictReadEvent = strictReadEvent;
    // A run outside a queue (a render effect's first run at creation) records
    // no event: its callback is the compiler's DOM writes.
    prevStrictRead = setStrictRead("an effect callback", queueEvent);
    setEffectCallback(true, queueEvent);
    enterCallback();
  }
  // Observe tier, like its `effectRunEnd` twin below: the frame the engine
  // opens here is what stamps the callback's writes as the effect's (the
  // cascade an observer reports) and what times the callback (the `effect`
  // record) — facts a production observer needs, not only a dev console.
  if (attrHooks !== null) attrHooks.effectRunStart(node);
  const prevCleanup = node._cleanup;
  node._cleanup = undefined;
  try {
    prevCleanup?.();
    const nextCleanup = node._effectFn(node._value, node._prevValue);
    if (true && nextCleanup !== undefined && typeof nextCleanup !== "function") {
      throw new Error(
        `${node._name || "effect"} callback returned an invalid cleanup value. Return a cleanup function or undefined.`
      );
    }
    // The final cleanup is invoked by disposeChildren at true disposal.
    node._cleanup = nextCleanup;
  } catch (error) {
    // (D-42, dev: as above — a boundary takes it or the system halts.)
    forgetThrownRead();
    ext(node)._error = new StatusError(node, error);
    node._statusFlags |= STATUS_ERROR;
    escalate(node, error);
  } finally {
    {
      setStrictRead(prevStrictRead, prevStrictReadEvent);
      setEffectCallback(false);
      exitCallback();
    }
    node._prevValue = node._value;
    node._modified = false;
    // The run applied: this is the frame now, so the dependency tail the
    // compute pass left linked goes (D-10, #3438 — `recompute` defers an
    // effect's trim while a run is owed; the twin of `commitPendingNode`'s
    // trim for a staged pass). An errored compute kept its full list with
    // `_depsTail` marking where it stopped; leave it, as the commit does.
    if (cleanPass) trimStaleDeps(node);
  }
  // Outside the try (see the rule in attribution-hooks.ts). Reached whether or
  // not the callback threw — a throw that escapes the catch above halts.
  if (attrHooks !== null) attrHooks.effectRunEnd(node);
}
GlobalQueue._runEffect = runEffect;
/**
 * Internal tracked effect - bypasses heap, goes directly to effect queue.
 * Runs as a leaf owner: child primitives and onCleanup are forbidden (true throws).
 * Uses stale reads.
 */
function trackedEffect(fn, options) {
  // A run owed at a hold's landing (#4607): `land`, made on first need, is
  // queued with the hold's stashed runs; `owed` dedups it, and a later run
  // that reads nothing held clears it.
  let owed = false,
    land;
  const run = () => {
    // `_modified` is NOT redundant with the heap: the heap dedups within a
    // pass, but several passes in one flush each enqueue `_run` into the same
    // user queue, and this gate is what collapses them into one run.
    if (!node._modified || node._flags & REACTIVE_DISPOSED) return;
    if (heldRunHook?.(node)) return;
    setTrackedQueueCallback(true);
    const epoch = notifyEpoch;
    owed = false;
    try {
      node._modified = false;
      recompute(node);
    } finally {
      setTrackedQueueCallback(false);
    }
    // A dep this run was served the committed value of (D-29) while a
    // different value is staged commits without a notify to it: its
    // subscriber walk predates the link. Re-run where that value commits
    // (F-1). Writes that net to the committed value stage no change.
    for (let d = node._deps; d !== null; d = d._nextDep) {
      const dep = d._dep;
      if (
        dep._pendingValue !== NOT_PENDING &&
        !(dep._config & CONFIG_OVERRIDE) &&
        (dep._pendingValue !== dep._value || !Object.is(dep._pendingValue, dep._value))
      ) {
        // Held: it commits at its hold's landing, which runs the hold's
        // stashed runs — but a run during the hold, for another dep, had
        // consumed the one queued for it (#4607). Re-run there, as a
        // stale reader would be (D-9); effect-phase code holds nothing (D-29).
        if (dep._config & CONFIG_HELD) {
          if (!owed) {
            owed = true;
            txOf(dep)._queues[EFFECT_USER - 1].push(
              (land ||= () => {
                if (owed) {
                  node._modified = true;
                  run();
                }
              })
            );
          }
        }
        // Staged this flush: the next round commits it. A memo is staged
        // by this run's own pull (`read` links, then pulls), whose walk
        // the recomputing run refuses (#4606); a signal written since the
        // epoch is the run's own write, never observed (#3006).
        else if (dep._notifiedAt <= epoch || dep._fn) {
          enqueueSub(node);
          schedule();
          // The re-run re-checks every dep, held ones included.
          break;
        }
      }
    }
  };
  const node = computed(
    () => {
      const prevCleanup = node._cleanup;
      node._cleanup = undefined;
      prevCleanup?.();
      const cleanup = staleValues(fn);
      if (cleanup !== undefined && typeof cleanup !== "function") {
        throw new Error(
          `${node._name || "trackedEffect"} callback returned an invalid cleanup value. Return a cleanup function or undefined.`
        );
      }
      node._cleanup = cleanup;
    },
    { ...options, lazy: true }
  );
  node._cleanup = undefined;
  node._config = (node._config & ~CONFIG_AUTO_DISPOSE) | CONFIG_CHILDREN_FORBIDDEN;
  node._modified = true;
  node._type = EFFECT_TRACKED;
  // Observe-tier label: the computed literal defaulted its `_name` slot to
  // "computed"; relabel by kind (a store into the slot, not a new field).
  if (options?.name === undefined) node._name = "trackedEffect";
  // Status dispatch rides the SHARED notifier (statusNotifierOf keys off
  // _type): its error arm is behavior-identical to the closure that used to
  // live here, without the per-node NodeExtension allocation.
  node._run = run;
  // The first run rides the heap like every wake (GlobalQueue._update), so a
  // tracked effect created inside a render-effect callback runs after that
  // pass's staged writes commit, not before.
  enqueueSub(node);
  schedule();
  if (!node._parent) {
    const message =
      "[NO_OWNER_EFFECT] Effects created outside a reactive context will never be disposed";
    reportDiagnostic(
      emitDiagnostic(
        {
          code: "NO_OWNER_EFFECT",
          kind: "lifecycle",
          severity: "warn",
          message,
          ownerId: node.id,
          ownerName: node._name,
          data: { effectType: "trackedEffect" }
        },
        node
      )
    );
  }
}
// Install the shared effect status notifier (statusNotifierOf serves it to
// every effect node) — module-scope: any bundle that creates effects
// evaluates this module.
setEffectStatusNotify(notifyEffectStatus);

let ambientHook;
const reported = new WeakSet();
/**
 * Registers the ambient client error hook — the one call a browser `init()`
 * makes to see every failure a boundary renders a fallback for, in
 * production. (Uncaught errors reach `reportError` / `window.onerror`.)
 *
 * ```ts
 * configureClientErrors({
 *   onError: (error, { ownerPath }) =>
 *     Sentry.captureException(error, {
 *       mechanism: { type: "solid.error_boundary", handled: true }
 *     })
 * });
 * ```
 */
function configureClientErrors(config) {
  if (config && config.onError !== undefined && typeof config.onError !== "function") {
    throw new TypeError(`Invalid onError: expected a function, received ${typeof config.onError}.`);
  }
  ambientHook = config ? config.onError : undefined;
}
/** The nearest root's hook above `owner` (parked under `ROOT_ERROR_HOOK`), else the ambient one. */
function hookFor(owner) {
  for (let o = owner; o; o = o._parent) {
    const hook = o[ROOT_ERROR_HOOK];
    if (hook !== undefined) return hook;
  }
  return ambientHook;
}
/** Component labels up the owner chain, root first — `_name` where the runtime keeps it. */
function labels(owner) {
  const path = [];
  for (let o = owner; o; o = o._parent) {
    const name = o._name;
    if (typeof name === "string" && name.length) path.push(name);
  }
  return path.length ? path.reverse() : undefined;
}
/**
 * Tells the client error hook about `error`, caught by the boundary whose
 * owner is `owner`, thrown by `thrower` (the computation the engine's status
 * wrapper named; unknown for a value that never crossed one) — once per
 * error object. A throwing hook is reported on the console and otherwise
 * ignored — a monitor must never take the app down.
 * @internal
 */
function reportClientError(error, owner, thrower) {
  const isObject = error !== null && (typeof error === "object" || typeof error === "function");
  if (isObject) {
    if (reported.has(error)) return;
    reported.add(error);
  }
  const hook = hookFor(owner);
  if (hook === undefined) return;
  const context = {};
  const boundary = labels(owner);
  const path = labels(thrower) ?? boundary;
  if (path !== undefined) context.ownerPath = path;
  if (boundary !== undefined) context.boundaryPath = boundary;
  try {
    hook(error, context);
  } catch (hookError) {
    console.error(hookError);
  }
}

/**
 * Ownership stamp (#3360): every backing the store ALLOCATES (CoW clones,
 * privatized committed backings) carries its owning target under this
 * enumerable symbol. One property write replaces the two weak-collection
 * registrations (ownership set + raw→target map) a fresh object used to pay
 * per draft — V8's identity-hash + ephemeron cost dominated the one-key
 * write floor. Enumerable so a spread copy (the plain-data clone path) stays
 * on the fast path and carries the stamp along.
 *
 * Owned backings are never user-reachable (`snapshot` copies them, the traps
 * hide the key), so every raw key walk in the store must skip `$OWNER`, and
 * ownership is answered by `isOwned` — a user object never carries it.
 * Overlay drafts (`Object.create(v)` over an owned `v`) inherit the stamp.
 */
const $OWNER = Symbol("STORE_OWNER");
/** raw → target for plain (family-less) stores. Family stores use their own
 * map so one raw can wrap once per family (§7b). */
const storeLookup = new WeakMap();
function setLookup(raw, t) {
  (t._fam?.map ?? storeLookup).set(raw, t);
}
function isOwned(raw) {
  return raw[$OWNER] !== undefined;
}
/** The target wrapping `raw` in `fam` (or the plain lookup). An owned raw
 * answers from its stamp; a user raw from the map. */
function lookupTarget(raw, fam) {
  const owner = raw[$OWNER];
  return owner !== undefined && owner._fam === fam ? owner : (fam?.map ?? storeLookup).get(raw);
}
/** Sticky descendants flag up the parent chain (§6d): adoption descends
 * into a changed child pair only where a target with nodes exists below. */
function markDescendants(target) {
  let t = target;
  while (t && !t.d) {
    t.d = true;
    t = t.u;
  }
}
/** Observe tier: the owner in scope at `createStore` (attribution names and
 * owner paths); the `_owner` write never reaches the proxy. */
const storeOwners = new WeakMap();

/** Brand keys the proxy answers directly (never user data). */
const $TRACK = Symbol("STORE_TRACK"),
  /** A store target's guess carrier (optimistic.ts, S2): one per lane. */
  $PATCH = Symbol("STORE_PATCH"),
  $TARGET = Symbol("STORE_TARGET"),
  $PROXY = Symbol("STORE_PROXY"),
  $RECORD = Symbol("VIEW_RECORD");
// ---------------------------------------------------------------------------
// raw marking (R42 / RUL-12: a value is never both wrapped and raw)
const rawValues = new WeakSet();
let rawValuesUsed = false;
function isRawValue(value) {
  return rawValuesUsed && rawValues.has(value);
}
/** Keep a value out of store wrapping: every store serves it as is. Pass a
 * class to keep all instances of it and its subclasses raw. */
function markRaw(value) {
  if (typeof value === "function") {
    if (value.prototype) wrappableProtos.set(value.prototype, false);
  } else if (isWrappable(value)) {
    rawValuesUsed = true;
    rawValues.add(value);
  }
  return value;
}
/** A shallow store's ingested record: raw by contract (values served
 * verbatim, slots replaced by reference). */
function markRawOne(v) {
  if (isWrappable(v)) {
    if (v[$TARGET] !== undefined) return;
    if (lookupTarget(v, null) !== undefined)
      throw new Error(
        "shallow store: an ingested record is already tracked as a deep store — one value cannot present both wrapped and raw"
      );
    rawValuesUsed = true;
    rawValues.add(v);
  }
}
function markRawIngest(container) {
  if (Array.isArray(container)) {
    for (let i = 0, len = container.length; i < len; i++) markRawOne(container[i]);
  } else {
    for (const k in container) markRawOne(container[k]);
  }
}
// ---------------------------------------------------------------------------
// wrappability
const OBJECT_PROTO = Object.prototype;
const wrappableProtos = new WeakMap();
function isWrappable(obj) {
  if (obj == null || typeof obj !== "object") return false;
  const proto = Object.getPrototypeOf(obj);
  if (proto === OBJECT_PROTO || proto === null) return true;
  if (Array.isArray(obj)) return true;
  let wrappable = wrappableProtos.get(proto);
  if (wrappable === undefined) {
    wrappable =
      Object.prototype.toString.call(obj) === "[object Object]" &&
      (typeof Node === "undefined" || !(obj instanceof Node)) &&
      isWrappable(proto);
    wrappableProtos.set(proto, wrappable);
  }
  return wrappable;
}
function ownEnumerableKeys(o) {
  return Reflect.ownKeys(o).filter(k => Object.prototype.propertyIsEnumerable.call(o, k));
}
// ---------------------------------------------------------------------------
// write override: a projection's draft ops carry it (the derive is the
// author — its writes must land outside the sync setter scope too, in a
// continuation after an `await`/`yield`). Projections return in S3; the
// traps already honour it.
let writeOverride = false;
function setWriteOverride(value) {
  writeOverride = value;
}
/** The list driver's view of a shallow store (map.ts): the backing a read of
 * every slot serves this reader, or `undefined` when the list is no shallow
 * store or a slot read could differ from it. Installed by store.ts's top
 * level, so a `<For>` in an app without stores carries none of it; it lives
 * beside `$TRACK` so map.ts reaches no other store module. */
let shallowView = null;
function installShallowView(view) {
  shallowView = view;
}

/**
 * Deferred hydration, dependency edges for stores. Only the frames client
 * imports this module, so nothing here ships without it.
 *
 * A plain store root (`createStore` over an object) is named like a plain
 * signal (core/identities.ts: `<owner id>~<n>`, on the same count), and the
 * whole store goes by that one name: the server records it for any read of
 * the store, at any depth, and a setter call on the store — on its root or
 * on a proxy nested in it — is a write to it. Its readers mount at the
 * setter's entry, before the draft runs, as a plain signal's do.
 */
/**
 * @internal The name of the store `x` belongs to — `x` a store proxy (a
 * setter's entry) or one of its property nodes (the setter notifying it) —
 * or undefined when it has none or it cannot be placed: a projection or
 * optimistic family, a store chained over another store's proxy, a root
 * named twice, or a proxy reached through a store other than the one it
 * was named as the root of.
 */
function storeName(x) {
  if (x === undefined) return undefined;
  let t = x._config & CONFIG_SLOT_NODE ? x._host : x[$TARGET];
  if (t === undefined) return undefined;
  // Up to the root; a target on the way named as a root of its own is one
  // object in two stores.
  while (t.u !== null) {
    if (t._fam !== null || t.ch || signalName(t) !== undefined) return undefined;
    t = t.u;
  }
  if (t._fam !== null || t.ch) return undefined;
  return signalName(t) || undefined;
}

// The guess layer: written optimistic values (F-11, D-33, D-34). lanes.ts
// holds what every lane needs; this module holds what only a guess reaches.
// Installed through `installGuessLayer` and `installGuessHooks`, and only
// this module sets CONFIG_GUESS, so a bundle without optimism drops it all.
/** Guesses written since the last seam, as `[node, value, question, …]`.
 * Applied at the seam: only then is it known whether there is a hold to
 * guess over (D-33). */
const pendingGuesses = [];
/** The lanes this seam's guesses read from (D-33). */
const readFrom = [];
/** What an optimistic updater composes on (F-10: the writer sees its own
 * unflushed writes). */
function guessedValueOf(n) {
  let g = pendingGuessOf(n);
  return g !== NOT_PENDING || (n._config & CONFIG_OVERRIDE && (g = latestOf(n)) !== NOT_PENDING)
    ? g
    : n._value;
}
function pendingGuessOf(n) {
  for (let i = pendingGuesses.length - 3; i >= 0; i -= 3)
    if (pendingGuesses[i] === n) return pendingGuesses[i + 1];
  return NOT_PENDING;
}
/** Guesses another action's lane took over, as `[node, guess, lane,
 * question, taker, …]`, kept to give back if the taker reverts (#4336). */
const kept = [];
/** `d`: taken without being written (a store target's other slots). */
function take(n, l, d) {
  const x = n._x;
  txOf(n);
  const v = latestOf(n);
  kept.push(n, v, x._transaction, x._q, l);
  rerun(n, l);
  if (x._shown) n._value = covered(n);
  covers.delete(n);
  x._shown = false;
  n._config &= d ? -10241 : ~CONFIG_GUESS;
  if (d) x._transaction = null;
  else {
    list(n, l);
    if (n._pendingValue === NOT_PENDING) setLaneValue(n, l, v);
  }
}
function optimisticWrite(n, v) {
  devWarnOptimisticWhileLoading(n);
  const prev = guessedValueOf(n);
  if (typeof v === "function") v = v(prev);
  // Equal is no guess, unless the node carries one: re-asking renews its question (#3347).
  if (n._equals && n._equals(prev, v) && !(n._config & CONFIG_GUESS)) return v;
  pendingGuesses.push(n, v, question || nextQuestion());
  if (attrHooks !== null) attrHooks.staged(n);
  for (let i = 0; i < readGuesses.length; i += 2)
    if (readGuesses[i + 1] === question) readFrom.push(readGuesses[i]);
  armGuessing();
  armGuessed();
  schedule();
  return v;
}
/** Each guess becomes its node's lane value. With no hold to guess over
 * and no flight of its own, the guess is dropped (D-33). */
function applyGuesses(parent) {
  let lane = null;
  for (let i = 0; i < pendingGuesses.length; i += 3) {
    const n = pendingGuesses[i];
    const v = pendingGuesses[i + 1];
    let changed = true;
    // A guess over a slot another frame holds entangles the frames (D-8, D-33).
    let holder = null;
    if (n._config & CONFIG_OVERRIDE) {
      changed = !n._equals || !n._equals(latestOf(n), v);
      if (!(n._config & CONFIG_GUESS)) {
        n._pendingValue = NOT_PENDING;
        n._config |= CONFIG_GUESS;
      } else if (n._x._shown) {
        // The screen keeps the shown guess until the lane's next reveal (F-1, S-30).
        screens.has(n) || screens.set(n, [n._value, n._x._transaction]);
        txOf(n)._reguessed = clock;
      }
      setLaneValue(n, txOf(n), v);
      holder = txOf(n)._parent;
    } else {
      let l;
      // Over a held node the guess composes on the held value, so it lives
      // until both holds end (F-11).
      if (n._config & CONFIG_HELD) l = newLane((holder = txOf(n)));
      else {
        if (parent === null && inFlight$1(n)) parent = newTransaction(false);
        if (parent === null) continue;
        l = lane ??= newLane(parent);
      }
      // Even a guess equal to the truth beneath it re-derives its readers in
      // the lane: a lane derivation must not show the hold's other writes (F-1).
      n._config |= CONFIG_OVERRIDE | CONFIG_GUESS;
      list(n, l);
      setLaneValue(n, l, v);
    }
    if (parent !== null && holder !== null) merge$1(parent, holder);
    ext(n)._q = pendingGuesses[i + 2];
    if (changed) {
      // As lane work, so a blocked lane's re-guess does not republish the committed view (#3460).
      laneDirty(n, txOf(n));
    }
  }
  pendingGuesses.length = 0;
}
/** `markJudged` first: a lane born at this seam has no work to judge yet. */
const applyGuessesAt = parent => {
  markJudged();
  if (pendingGuesses.length !== 0) {
    // A guess written after reading another hold's guess entangles the holds (D-33).
    if (parent !== null)
      for (const r of readFrom) if (lanes.includes(r)) merge$1(parent, r._parent);
    applyGuesses(parent);
  }
  // Keep reads of live lanes: the reading action may guess after a yield.
  let k = 0;
  for (let i = 0; i < readGuesses.length; i += 2)
    if (lanes.includes(readGuesses[i]))
      ((readGuesses[k++] = readGuesses[i]), (readGuesses[k++] = readGuesses[i + 1]));
  readFrom.length = 0;
  readGuesses.length = k;
};
/** The correction is a revert (no truth landed beneath): the lane gives
 * back the guesses it took (#4336). */
let reverting = false;
/** A guess's truth arrived: a different one corrects, an equal one confirms
 * (F-11, D-34). The truth is held by the guess's hold, not by the tick that
 * carried it, so one landing answering two actions' guesses does not
 * entangle them (D-33). */
function supersede(n, value, changed) {
  const l = txOf(n);
  // The parent may have merged since the lane opened (D-33).
  const parent = resolveTx(l._parent ?? l);
  // A new server object that reconciles to the guessed row unchanged confirms (#3898).
  if (changed && n._config & CONFIG_SLOT_NODE && GlobalQueue._echoes?.(latestOf(n), value))
    changed = false;
  if (changed) {
    dissolveLane(l, parent, n);
    if (reverting) marked$1.add(l);
    // A shown guess stays on screen until the commit (D-34), so the
    // published-inputs mark no longer applies (#4795).
    if (l._shown) (marked$1.add(n), (n._config &= ~CONFIG_INPUTS_PUBLISHED));
    n._config &= -10241;
    n._x._shown = false;
    covers.delete(n);
    holdNode(n, parent);
    n._pendingValue = value;
    if (n._subs !== null) insertSubs(n);
  } else {
    landBeneath(n, value);
    GlobalQueue._confirmed?.(l);
  }
}
/** Body-end correction (D-34): once the action body has returned and no
 * truth is in flight, the truth at hand judges each guess. Runs before the
 * park decision so the correction's flights are the parent's (#3409). True
 * when a lane was judged. */
function laneCorrections() {
  let judged = false;
  const live = lanes.slice();
  for (let k = 0; k < live.length; k++) {
    const l = live[k];
    if (lanes.indexOf(l) === -1) continue;
    const p = resolveTx(l._parent);
    if (!p._acted || p._open !== 0 || p._lane || ownFlights(p) || guessFlights(l)) continue;
    judged = true;
    // A correction dissolves the lane, so re-check membership each step.
    for (let i = 0; i < l._nodes.length && lanes.indexOf(l) !== -1; i++) {
      const n = l._nodes[i];
      if (n._config & CONFIG_GUESS && n._x._transaction === l) {
        const truth = underOf(n);
        reverting = n._pendingValue === NOT_PENDING;
        supersede(n, truth, !n._equals || !n._equals(latestOf(n), truth));
        reverting = false;
      }
    }
  }
  return judged;
}
/** A write landing on a guess. False when a store slot's guess is taken by
 * another action's lane instead (#4336). */
function guessWrite(el, v) {
  if (GlobalQueue._slotTake?.(el)) return false;
  // Provenance (D-34): an older question's answer is held beneath, not judged.
  const older = stale(el, question);
  if (GlobalQueue._laneRebase?.(el, v, older) || older) landBeneath(el, v);
  else supersede(el, v, !el._equals || !el._equals(latestOf(el), v));
  schedule();
  return true;
}
/** A guess's own pass finished: whatever recomputes the node is its
 * source (D-34). An errored pass stages nothing; a stale answer is held
 * beneath (provenance, D-34). */
function laneOutcome(el, value, errored) {
  if (errored) return true;
  if (stale(el, answered(el))) {
    landBeneath(el, value);
    return true;
  }
  supersede(el, value, !el._equals || !el._equals(latestOf(el), value));
  return true;
}
/** Provenance (D-34): unstamped (0) answers are current. */
function stale(el, q) {
  return q !== 0 && q < el._x._q;
}
/** The newest question among the sources that changed this round; 0 when none is stamped. */
function answered(el) {
  let q = 0;
  for (let l = el._deps; l !== null; l = l._nextDep) {
    const d = l._dep;
    if (d._x !== null && d._x._q > q && d._time === clock) q = d._x._q;
  }
  return q;
}
/** A revert landing gives back the guesses its lane took from an action
 * still out, in the landing's frame (F-1, #4336). */
function revertLanding(u) {
  for (let i = kept.length - 5; i >= 0; i -= 5)
    if (resolveTx(kept[i + 4]._parent) === u) {
      let [n, v, g, q, t] = kept.splice(i, 5);
      if (
        marked$1.has(t) &&
        lanes.includes(g) &&
        (!(n._config & CONFIG_SLOT_NODE) || (n = GlobalQueue._slotTake(n, true)))
      ) {
        const x = ext(n);
        if (x._transaction === null || txOf(n) === u) {
          commitPendingNode(n);
          x._q = q;
          n._config |= CONFIG_OVERRIDE | CONFIG_GUESS;
          list(n, g);
          setLaneValue(n, g, v);
          rerun(n, u);
        }
      }
    }
}
/** A re-asked shown guess: what the screen showed, and its lane, until the next reveal (#4775). */
const screens = /* @__PURE__ */ new WeakMap();
installGuessLayer(take, guessWrite, revertLanding, (el, revealed) => {
  const s = screens.get(el);
  if (revealed) screens.delete(el);
  return s && !revealed && el._config & CONFIG_GUESS && el._x._shown && el._x._transaction === s[1]
    ? s[0]
    : el._value;
});
installGuessHooks(laneOutcome, applyGuessesAt, () => lanes.length !== 0 && laneCorrections());

/**
 * Low-level reactive-cleanup primitive. Registers a callback that runs when
 * the surrounding owner is disposed.
 *
 * **In 2.0 user code this is rare.** The two cases where you might reach for
 * it have better-shaped tools:
 *
 * - **Component lifecycle (mount/unmount, listeners, intervals):** use
 *   {@link onSettled} and **return** a cleanup function. Setup and teardown
 *   stay paired in one block. This replaces the 1.x `onMount` + `onCleanup`
 *   pairing.
 * - **Cleanup tied to an effect run:** `onCleanup` does not belong in
 *   `createEffect`'s apply phase. If a compute phase genuinely needs per-run
 *   teardown, that's usually a sign the work should be a memo/projection
 *   instead, or moved to `onSettled` if it's lifecycle-shaped.
 *
 * Where `onCleanup` is the right tool is **library / custom-primitive
 * internals** — coordinating disposal inside a `createRoot` body, or wiring
 * cleanup to a captured owner via `runWithOwner` from a custom factory.
 * Application code rarely needs to write any of those shapes directly.
 *
 * Must be called inside an owner. Calling outside an owner is a no-op (with a
 * dev-mode warning).
 *
 * Cannot be used inside `createTrackedEffect` or `onSettled` — return a
 * cleanup function from the callback body instead.
 *
 * Cleanups run in unwind order: an owner's children are disposed before its
 * own cleanups, and within one owner later registrations run before earlier
 * ones. In production a component body shares its enclosing owner, so
 * register cleanup before creating children when the order between them
 * matters.
 *
 * @example
 * ```ts
 * // Library shape: thread a resource's disposal into a *captured* owner
 * // from a factory that has no settle-phase setup of its own. `onSettled`
 * // would queue a callback we don't need; `onCleanup` is the leaner
 * // primitive when the only job is "register disposal on this owner".
 * function bindToOwner<T extends { dispose(): void }>(owner: Owner, resource: T): T {
 *   runWithOwner(owner, () => onCleanup(() => resource.dispose()));
 *   return resource;
 * }
 * ```
 */
function onCleanup(fn) {
  {
    const owner = getOwner();
    if (!owner) {
      const message =
        "[NO_OWNER_CLEANUP] onCleanup called outside a reactive context will never be run";
      reportDiagnostic(
        emitDiagnostic({
          code: "NO_OWNER_CLEANUP",
          kind: "lifecycle",
          severity: "warn",
          message
        })
      );
    } else if (owner._config & CONFIG_CHILDREN_FORBIDDEN || inRefCallback()) {
      const message = inRefCallback()
        ? "[CLEANUP_IN_FORBIDDEN_SCOPE] Cannot use onCleanup inside a ref callback; register it in the factory that returns the ref (the component body)"
        : "[CLEANUP_IN_FORBIDDEN_SCOPE] Cannot use onCleanup inside createTrackedEffect or onSettled; return a cleanup function instead";
      emitDiagnostic({
        code: "CLEANUP_IN_FORBIDDEN_SCOPE",
        kind: "lifecycle",
        severity: "error",
        message,
        ownerId: owner.id,
        ownerName: owner._name
      });
      throw new Error(message);
    }
  }
  return cleanup(fn);
}
function accessor(node) {
  const fn = read.bind(null, node);
  fn[$REFRESH] = node;
  return fn;
}
function createSignal(first, second) {
  if (typeof first === "function") {
    const node = computed(first, second);
    node._config &= ~CONFIG_AUTO_DISPOSE;
    return [accessor(node), setMemo.bind(null, node)];
  }
  const node = signal(first, second);
  registerGraph(node, getOwner());
  if (signalHook !== null) signalHook(node);
  return [accessor(node), setSignal.bind(null, node)];
}
function createOptimistic(first, second) {
  if (typeof first === "function") {
    const node = computed(first, second);
    node._config &= ~CONFIG_AUTO_DISPOSE;
    return [accessor(node), optimisticWrite.bind(null, node)];
  }
  const node = signal(first, second);
  registerGraph(node, getOwner());
  return [accessor(node), optimisticWrite.bind(null, node)];
}
function createMemo(compute, options) {
  return accessor(computed(compute, options));
}
/**
 * Creates a reactive effect with **separate compute and effect phases**.
 *
 * - `compute(prev)` runs reactively — *put all reactive reads here*. The
 *   returned value is passed to `effect` and is also the new "previous" value
 *   for the next run.
 * - `effect(next, prev?)` runs imperatively (untracked) after the queue
 *   flushes. *Put DOM writes / fetch / logging / subscriptions here.* It may
 *   return a cleanup function which runs before the next effect or on
 *   disposal.
 *
 * Reactive reads inside `effect` will *not* re-trigger this effect — that's
 * intentional. If you need a single-phase tracked effect, use
 * `createTrackedEffect` (with the tradeoffs noted there).
 *
 * Pass an `EffectBundle` (`{ effect, error }`) instead of a plain function to
 * intercept **compute-phase** errors — errors thrown by `compute` or arriving
 * from upstream reactive sources (including async rejections), which your own
 * code has no frame to `try/catch`. The `error` handler is the error arm of
 * the effect phase: it runs on the same schedule and in the same imperative,
 * writable scope as `effect` (setting error state via signals is fine), and
 * only for *settled* errors — a transient error that recovers before the
 * effect phase runs `effect` with the recovered value instead, and a held
 * transition defers it exactly as it defers `effect`. Without an `error`
 * handler a compute-phase error is logged and the effect simply skips that
 * run — a non-render effect's reactivity failing does not crash the app.
 * Rethrowing from `error` escalates it to the nearest error boundary
 * (halting the system if none exists).
 *
 * The **effect phase is different**: it is your own imperative code, so handle
 * failures with `try/catch` where they occur. An uncaught effect-phase throw
 * is treated as an unhandled application error — caught by the nearest
 * `createErrorBoundary`/`<Errored>`, and permanently halting the reactive
 * system if there is none. It is *not* routed to the bundle's `error` handler.
 *
 * ```typescript
 * createEffect<T>(compute, effectFn | { effect, error }, options?: EffectOptions);
 * ```
 * @param compute a function that receives its previous value and returns a new value used to react on a computation
 * @param effectFn a function that receives the new value and is used to perform side effects (return a cleanup function), or an `EffectBundle` with `effect` and `error` handlers
 * @param options `EffectOptions` -- name, defer, schedule, transparent
 *
 * @example
 * ```ts
 * const [count, setCount] = createSignal(0);
 *
 * createEffect(
 *   () => count(),                  // compute: tracks `count`
 *   value => console.log(value)     // effect: side effect
 * );
 *
 * setCount(1); // logs 1 after the next flush
 * ```
 *
 * @example
 * ```ts
 * createEffect(
 *   () => userId(),
 *   id => {
 *     const ctrl = new AbortController();
 *     fetch(`/users/${id}`, { signal: ctrl.signal });
 *     return () => ctrl.abort(); // cleanup before next run / disposal
 *   }
 * );
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/reactivity/create-effect
 */
function createEffect(compute, effectFn, options) {
  if (effectFn === undefined) {
    const message =
      "[MISSING_EFFECT_FN] createEffect requires both a compute function and an effect function. " +
      "Use `createEffect(() => signal(), value => doWork(value))`. " +
      "If you want a derived value, use `createMemo`. " +
      "If you want a one-shot side effect, just call the function directly.";
    emitDiagnostic({
      code: "MISSING_EFFECT_FN",
      kind: "lifecycle",
      severity: "error",
      message
    });
    throw new Error(message);
  }
  effect(compute, effectFn.effect || effectFn, effectFn.error, {
    user: true,
    ...options
  });
}
/**
 * Creates a reactive computation that runs during the render phase as DOM elements
 * are created and updated but not necessarily connected.
 *
 * Same compute / effect split as `createEffect`, but scheduled inside the render
 * queue rather than after it. Reach for this only when authoring renderer
 * plumbing (custom DOM bindings, JSX-generated `insert()` / `spread()` calls).
 * App code should use `createEffect`.
 *
 * ```typescript
 * createRenderEffect<T>(compute, effectFn, options?: EffectOptions);
 * ```
 * @param compute a function that receives its previous value and returns a new value used to react on a computation
 * @param effectFn a function that receives the new value and is used to perform side effects
 * @param options `EffectOptions` -- name, defer, schedule, transparent
 *
 * @example
 * ```ts
 * // Custom directive: bind an element's textContent to a reactive source.
 * function bindText(el: HTMLElement, source: () => string) {
 *   createRenderEffect(
 *     () => source(),
 *     value => { el.textContent = value; }
 *   );
 * }
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/advanced/specialized-reactivity/create-render-effect
 */
function createRenderEffect(compute, effectFn, options) {
  effect(compute, effectFn, undefined, options);
}
/**
 * Creates a tracked reactive effect where dependency tracking and side effects happen
 * in the same scope.
 *
 * @deprecated Do not use in new code. For a side effect that follows reactive
 * state, use `createEffect(compute, effect)` — it separates tracking from the
 * side effect, knows its dependencies before it runs, and participates in
 * async and transitions. For one-time DOM work after render (measuring,
 * attaching third-party widgets to a ref), use `onSettled`. Tracking from
 * inside the effect phase — the only thing this primitive adds — is retained
 * solely to ease 1.x migration: it runs beside user-effect callbacks after
 * values commit, never holds a transition, and cannot observe a write staged
 * earlier in the same flush by a signal it has not read yet.
 *
 * WARNING: Because tracking and effects happen in the same scope, this primitive
 * may run multiple times for a single change or show tearing (reading inconsistent
 * state). Use only when dynamic subscription patterns require same-scope tracking.
 *
 * The callback runs during the flush itself: writes made inside it are queued
 * into the same flush's continuation and are never visible to the callback's
 * own reads (reads return settled values, as in every effect-phase scope), and
 * `flush()` cannot be called from inside it (dev throws; production is a
 * no-op) — defer with `queueMicrotask(() => flush())` if needed.
 *
 * ```typescript
 * createTrackedEffect(compute, options?: { name?: string });
 * ```
 * @param compute a function that contains reactive reads to track and returns an optional cleanup function to run on disposal or before next execution
 * @param options -- name
 *
 * @example
 * ```ts
 * createTrackedEffect(() => {
 *   const target = focusedNode();
 *   if (!target) return;
 *
 *   const handler = () => log(target.value());
 *   target.on("change", handler);
 *
 *   return () => target.off("change", handler);
 * });
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/advanced/specialized-reactivity/create-tracked-effect
 */
function createTrackedEffect(compute, options) {
  trackedEffect(compute, options);
}
/**
 * Creates a reactive computation that runs after the render phase with flexible tracking.
 *
 * ```typescript
 * const track = createReaction(effectFn, options?: EffectOptions);
 * track(() => { // reactive reads });
 * ```
 * @param effectFn a function (or `EffectBundle`) that is called when tracked function is invalidated
 * @param options `EffectOptions` -- name, defer
 *
 * @example
 * ```ts
 * const [count, setCount] = createSignal(0);
 *
 * const track = createReaction(() => {
 *   console.log("count changed once, re-arm to listen again");
 *   track(() => count()); // re-arm
 * });
 *
 * track(() => count()); // initial arm
 *
 * setCount(1); // logs once, reaction re-armed for next change
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/advanced/specialized-reactivity/create-reaction
 */
function createReaction(effectFn, options) {
  let cl = undefined;
  cleanup(() => cl?.());
  const owner = getOwner();
  // The currently armed effect node. `track()` replaces the previous
  // subscription (1.x semantics): without disposing the superseded arm, its
  // sources stayed live (firing the callback for replaced dependencies), each
  // accumulated arm delivered its own fire, and un-fired arms leaked as live
  // effect nodes until the owner disposed (#2861).
  let arm;
  return tracking => {
    if (arm) {
      dispose(arm);
      arm = undefined;
    }
    runWithOwner(owner, () => {
      effect(
        () => {
          const node = getOwner();
          arm = node;
          tracking();
          return node;
        },
        node => {
          arm = undefined;
          cl?.();
          const cleanup = (effectFn.effect || effectFn)?.();
          if (true && cleanup !== undefined && typeof cleanup !== "function") {
            throw new Error(
              "Reaction callback returned an invalid cleanup value. Return a cleanup function or undefined."
            );
          }
          cl = cleanup;
          dispose(node);
        },
        effectFn.error,
        {
          ...options,
          user: true,
          defer: true
        }
      );
    });
  };
}
/** A promise over a reactive expression (`resolve`, `until`): a pass that
 * runs the expression and settles on a microtask. Not an effect — an
 * effect's run is the flush's effect phase, and a flush parked by the
 * action that is `yield`ing on this promise stashes its runs with the
 * transaction (#2930); the heap runs a pass parked or not. A pending read
 * re-runs it when the source lands (the pass is pending itself); the
 * expression throwing settles the rejection. */
/** The promise-delivery readers' pass (`resolve`, `until`, `refresh`'s
 * waiter): delivers what it read on a microtask. By posture (#3482, #3490):
 * - the reader of the transaction it was created in (`own`: an action's
 *   body, a `refresh()` called there — the waiter reads its slice's frame on
 *   a later microtask) delivers that frame — the staged landing an action
 *   holds (the action awaits the promise that would otherwise wait on its
 *   own settle);
 * - a mainline reader whose pass read a frame another transaction holds
 *   joined it (L2, born held — F-7) and delivers at that commit: the
 *   committed view, never a foreign hold's unrevealed frame. */
function watch(fn, onValue, onError, config = 0, inPass, own = flushTransaction) {
  if (config & CONFIG_AUTHORITATIVE) {
    GlobalQueue._truth = truthRead;
    GlobalQueue._confirmed = truthConfirmed;
  }
  // `config` is on the node for its first pass too (the pass runs inside
  // `computed()`): `until`'s predicate is authoritative from its first read,
  // or an already-displayed guess would satisfy it in a later slice. (REDERIVE
  // after: the first pass's own error is the promise's rejection.)
  computed(
    () => {
      let v,
        failed = false;
      try {
        v = fn();
      } catch (e) {
        if (e instanceof NotReadyError) throw e;
        // An error is an answer (D-41): delivered by the same frame rule.
        // (D-42, dev: the runtime's catch, not a user's.)
        forgetThrownRead();
        v = unwrapStatusError(e);
        failed = true;
      }
      const c = getOwner();
      // A truth reader waits out a `createDeferred` flight its value derives
      // from, which serves everyone else the stale answer (D-40).
      const lag = config & CONFIG_AUTHORITATIVE && GlobalQueue._lagging?.(c);
      if (lag) throw new NotReadyError(lag);
      // (Outside a flush a mainline pass joins through `passTx`.)
      const joined = flushTransaction ?? passTx;
      if (
        c._flags & REACTIVE_JOINED &&
        joined !== null &&
        (own === null || resolveTx(own) !== resolveTx(joined))
      ) {
        staleReader(c, resolveTx(joined));
        return;
      }
      if (failed) return void queueMicrotask(() => onError(v));
      inPass?.(v);
      queueMicrotask(() => onValue(v));
    },
    { _extraConfig: config }
  )._config |= CONFIG_REDERIVE;
}
/**
 * Awaits a reactive expression and returns its first fully-settled value as a
 * `Promise`. Pending async reads (`createMemo` returning a promise, etc.) are
 * waited on; once the expression returns synchronously without `NotReadyError`
 * the promise resolves with that value. If the expression settles with an
 * error instead — including an async source that rejects — the promise
 * rejects with it.
 *
 * Must be called *outside* a tracking scope — it doesn't subscribe, it just
 * resolves the current value once.
 *
 * @example
 * ```ts
 * const user = createMemo(() => fetch(`/users/${id()}`).then(r => r.json()));
 *
 * // outside any reactive scope
 * const initial = await resolve(() => user());
 * ```
 *
 * @param fn a reactive expression to resolve
 */
function resolve(fn) {
  if (getObserver()) {
    throw new Error(
      "Cannot call resolve inside a reactive scope; it only resolves the current value and does not track updates."
    );
  }
  return new Promise((res, rej) => {
    createRoot(dispose =>
      watch(
        fn,
        value => (res(value), dispose()),
        err => (rej(err), dispose())
      )
    );
  });
}
/**
 * Invalidates one reactive source, forcing it to re-execute even if its inputs
 * haven't changed, and returns a promise for the target's NEXT QUIESCENT
 * STATE — the re-ask (and anything that supersedes it) has settled.
 *
 * Pass either a Solid-created accessor or a projected store created from
 * `createStore(fn, ...)` / `createProjection(...)`. `refresh()` is a
 * write-like invalidation operation: it does not read the target's value, and
 * refreshing a plain signal accessor is a no-op that resolves immediately.
 *
 * The returned promise is safe to ignore (fire-and-forget refresh is
 * unchanged, and a failed refetch will not surface an unhandled rejection).
 * Awaiting it gives imperative flows the settle point without a reactive
 * read:
 * - Accessor targets resolve with the settled value; store targets resolve
 *   with the store node passed (reads through it are fresh after the await).
 * - A failed re-ask rejects with the error.
 * - Semantics are quiescence, not flight identity: if another refresh (or
 *   any invalidation) supersedes this one mid-flight, the promise waits for
 *   — and delivers — whatever finally lands.
 *
 * @example
 * ```ts
 * const user = createMemo(async () => fetch(`/users/${id()}`).then(r => r.json()));
 *
 * // Fire-and-forget re-fetch
 * <button onClick={() => refresh(user)}>Reload</button>;
 *
 * // Imperative settle point
 * const fresh = await refresh(user);
 * ```
 */
function refresh(target) {
  const node = target?.[$REFRESH];
  if (!node) {
    {
      const message =
        "[INVALID_REFRESH_TARGET] refresh() expects a Solid source accessor or refreshable store. " +
        "Pass the original source target, not a wrapper function or derived property read.";
      emitDiagnostic({
        code: "INVALID_REFRESH_TARGET",
        kind: "write",
        severity: "error",
        message
      });
      throw new Error(message);
    }
  }
  // Mark now, watch on a microtask. The waiter is resolve()'s machinery with
  // two extra reader bits, but it must NOT compute at call time (effects
  // recompute eagerly on creation): same-tick refreshes coalesce into ONE
  // re-ask only because every mark lands before anything pulls, and eager
  // per-call pulls turned three refreshes into three fetches. Deferred, the
  // waiter's first read sees the coalesced state: FRESH_READ pulls the node
  // through recompute if it is still dirty (self-deduping — a clean node
  // no-ops, so N waiters cost one pull; this also closes the race where a
  // waiter reads the PRE-re-ask value as settled and delivers stale), after
  // which the read either parks on the re-ask's pending window (async — the
  // settle walk re-runs it on every landing, equal-value included, and a
  // rejection arrives through the effect's error channel) or serves the sync
  // answer. resolve()'s own eager compute is untouched: created after a
  // refresh it still settles stale-while-revalidate (#2930) — its contract is
  // "first settled value", not "next quiescent state".
  markRefresh(node);
  // The transaction `refresh()` is called in (an action's slice): the
  // waiter, created on the microtask below, is its reader (`watch`).
  const own = flushTransaction;
  const promise = new Promise((res, rej) => {
    queueMicrotask(() => {
      // The root is load-bearing: an ownerless computed is auto-disposing,
      // and the settle walk releases an unobserved one instead of re-running
      // it, so the waiter would never see the refetch land (#3888). Settle
      // disposes the waiter directly; the capture runs inside the watcher's
      // own pass, where the ambient owner IS the watcher — exactly what
      // dispose() takes.
      let waiter = null;
      const make = () =>
        watch(
          () => {
            if (waiter === null) waiter = getOwner();
            // Authoritative: the truth beneath a guess (`until`'s view). A
            // guessed node's own re-ask is not answered by it (#3895): park
            // on that flight, or reject with its error.
            const v = read(node);
            if (node._config & CONFIG_GUESS && node._statusFlags & (STATUS_PENDING | STATUS_ERROR))
              throw node._x?._error;
            return v;
          },
          value => {
            res(typeof target === "function" ? value : target);
            dispose(waiter);
          },
          err => {
            rej(err);
            dispose(waiter);
          },
          CONFIG_FRESH_READ | CONFIG_AUTHORITATIVE,
          undefined,
          own
        );
      createRoot(make);
    });
  });
  // Fire-and-forget refresh must not turn a failed refetch into an unhandled
  // rejection; awaiting callers attach their own handlers to `promise`.
  promise.catch(() => {});
  return promise;
}
/**
 * Awaits a reactive predicate and resolves the first time it settles *truthy*,
 * with that (narrowed) value. Falsy results and pending async reads both mean
 * "not yet": the subscription stays live and re-evaluates as sources change.
 * If the predicate settles with an error — a throw, or an async source that
 * rejects — the promise rejects with it, as do timeout and abort.
 *
 * Where {@link resolve} answers "what is this value" (first settled value,
 * whatever it is), `until` answers "when does the world confirm this
 * condition".
 *
 * This is the acknowledgment mechanism for mutations confirmed on a live data
 * channel (sockets, subscriptions, live queries) rather than by the mutation's
 * own response: correlate by a client-generated id or version in the predicate,
 * and let truth arrive however it arrives — push, refetch, or another tab.
 *
 * Must be called *outside* a tracking scope.
 *
 * @example
 * ```ts
 * const clientId = crypto.randomUUID();
 * await socket.send({ clientId, text }); // fire-and-forget transport
 * // Wait until the live source echoes the write:
 * await until(() => messages.some(m => m.clientId === clientId), { timeout: 10_000 });
 * ```
 *
 * @param fn a reactive predicate
 * @param options optional `timeout` (ms) and abort `signal`
 */
function until(fn, options) {
  if (getObserver()) {
    throw new Error(
      "Cannot call until inside a reactive scope; await it from an action or another imperative scope."
    );
  }
  return new Promise((res, rej) => {
    const signal = options?.signal;
    if (signal?.aborted) return rej(signal.reason);
    // The awaiting action's transaction, if any (its body runs joined to
    // it). `until()` is a declaration of relatedness (#3164 follow-up): the
    // predicate names the condition that confirms the action, so the frame
    // whose landing flips it truthy is the confirming event by the user's
    // own definition — it reveals WITH the action's settle, not before
    // (`saving=true` beside the confirmed `version=1` is a tear no timeline
    // contains). Updates that do not flip the predicate reveal freely.
    const t = flushTransaction;
    createRoot(dispose => {
      let timer;
      let onAbort;
      const settle = fire => {
        if (timer !== undefined) clearTimeout(timer);
        if (onAbort !== undefined) signal.removeEventListener("abort", onAbort);
        fire();
        dispose();
      };
      // Authoritative: the predicate sees the truth, never the action's own
      // optimistic guess — a guess cannot satisfy the wait for its own
      // confirmation.
      watch(
        fn,
        value => {
          // Falsy is "not yet": keep the subscription live and wait for the
          // next evaluation. Only a truthy settled value resolves.
          if (value) settle(() => res(value));
        },
        err => settle(() => rej(err)),
        CONFIG_AUTHORITATIVE,
        value => {
          // The flip, inside the pass of the flush carrying it: the flush
          // joins the action's transaction, and what it staged is held and
          // revealed with the action.
          if (value && t !== null && globalQueue._running && resolveTx(t)._open !== 0)
            joinFuture(t);
        }
      );
      if (options?.timeout !== undefined)
        timer = setTimeout(() => settle(() => rej(new TimeoutError())), options.timeout);
      if (signal !== undefined) {
        onAbort = () => settle(() => rej(signal.reason));
        signal.addEventListener("abort", onAbort, { once: true });
      }
    });
  });
}
/**
 * Schedules `callback` to run **once** after the reactive graph has fully
 * settled — i.e. once every pending async read inside the current owner has
 * resolved and the queue has flushed. Each call registers a single fire; it
 * does not create an ongoing subscription.
 *
 * The canonical lifecycle primitive in 2.0. Three main usages:
 *
 * - **Component-level setup-and-teardown** *(the most common shape)*: run
 *   setup after the component's first stable render and **return a cleanup
 *   function** to dispose it on owner disposal. This is the replacement for
 *   the 1.x `onMount` + `onCleanup` pairing — setup and teardown live in one
 *   block, and `onCleanup` is no longer the right tool for component
 *   bodies. (`onMount` no longer exists in 2.0.)
 * - **Post-settle "ready" hook:** run once after a component's first stable
 *   render — analytics ping, focus, scroll-into-view, etc. No cleanup needed.
 * - **Inside an event handler:** schedule work to run after the action /
 *   transition triggered by the event has completed.
 *
 * Reactive reads inside the callback are *not* tracked — to react to
 * subsequent settles, register a new `onSettled` each time.
 *
 * The callback runs during the settle flush itself, which gives it the same
 * write semantics as every other effect-phase scope (the effect half of
 * `createEffect`, event handlers):
 *
 * - **Writes** are queued into the same flush's continuation — dependent memos
 *   and effects update before the flush returns — but reads inside the
 *   callback keep returning the settled (pre-write) values. A callback never
 *   observes its own unsettled write. Functional setters still compose:
 *   `set(v => v + 1)` twice increments twice.
 * - **`flush()` cannot be called** from inside the callback — the flush is
 *   already running (dev throws; production is a no-op). To force a drain
 *   after this settle, defer it: `queueMicrotask(() => flush())`.
 *
 * `onCleanup` is **not** allowed inside the callback — return a cleanup
 * function instead. The returned cleanup runs on owner disposal.
 *
 * A cleanup return is only honored when `onSettled` is called from an **owned**
 * scope (e.g. a component body). When it fires out of band from an *unowned*
 * scope — an event handler, a tracked effect, or another `onSettled` — there is
 * no owner lifecycle to bind a cleanup to; returning one is a dev-mode error
 * (and is dropped in production). Use the post-settle/event-handler forms below
 * for one-shot work, and keep setup-with-teardown in an owned scope.
 *
 * @example
 * ```tsx
 * // Component-level setup + teardown — replaces onMount + onCleanup.
 * // Subscribe to an external source on mount, unsubscribe on dispose.
 * function useViewportWidth() {
 *   const [width, setWidth] = createSignal(window.innerWidth);
 *   onSettled(() => {
 *     const onResize = () => setWidth(window.innerWidth);
 *     window.addEventListener("resize", onResize);
 *     return () => window.removeEventListener("resize", onResize);
 *   });
 *   return width;
 * }
 * ```
 *
 * @example
 * ```tsx
 * // Post-settle "ready" hook — no cleanup needed.
 * function Dashboard() {
 *   const data = createMemo(async () => fetchData());
 *
 *   onSettled(() => {
 *     analytics.track("dashboard.ready");
 *   });
 *
 *   return <Loading fallback={<Spinner />}><pre>{data()}</pre></Loading>;
 * }
 * ```
 *
 * @example
 * ```tsx
 * // Event-handler — runs after the action settles.
 * function SaveButton() {
 *   const save = action(function* () {
 *     yield api.save();
 *   });
 *
 *   const handleClick = () => {
 *     save();
 *     onSettled(() => toast("Saved!"));
 *   };
 *
 *   return <button onClick={handleClick}>Save</button>;
 * }
 * ```
 *
 * @param callback Function to run; may return a cleanup function that fires
 *   on owner disposal
 */
function onSettled(callback) {
  const owner = getOwner();
  owner && !(owner._config & CONFIG_CHILDREN_FORBIDDEN)
    ? trackedEffect(() => untrack(callback), { name: "onSettled" })
    : globalQueue.enqueue(EFFECT_USER, function fire() {
        // Settled means derived. A settle that reverts optimism (or replays
        // gated reads) only enqueues the affected subscribers; the pass after
        // the commit re-derives them. Fired in the commit pass, the callback
        // read the optimistic source already reverted beside a sync memo of it
        // still holding the optimistic value — reads do not pull (#3411). Fall
        // to the next pass while the heap has work; `run` swapped the queue,
        // so this lands there, and `enqueue` keeps the drain alive.
        if (dirtyQueue._max >= dirtyQueue._min) return globalQueue.enqueue(EFFECT_USER, fire);
        // Unowned, out-of-band fire (no owner, or a children-forbidden one this
        // one-shot must not bind to): a returned cleanup has no lifecycle to
        // attach to. Reject it in dev; in production the return is simply
        // dropped — never bound to an unrelated owner or run eagerly.
        const cleanup = callback();
        if (cleanup !== undefined) {
          const message =
            "[SETTLED_CLEANUP_UNOWNED] onSettled returned a cleanup in an unowned scope; a cleanup can only be honored under an owner. Call your setup helper from an owned scope (e.g. the component body) instead of from inside an event handler, tracked effect, or another onSettled.";
          emitDiagnostic({
            code: "SETTLED_CLEANUP_UNOWNED",
            kind: "lifecycle",
            severity: "error",
            message
          });
          throw new Error(message);
        }
      });
}
/**
 * Schedules `callback` to run **once**, right after the pending flush
 * finishes, in the same microtask checkpoint (before the browser paints).
 * Unlike `onSettled`, it never waits on async work: if that flush is held by
 * a pending transition, the callback still runs.
 *
 * Its main use is keeping a write out of another update's hold. Writes made
 * in the same synchronous frame are applied together, so when one of them
 * starts async work (a route change, a fetch), the others are held with it
 * until it resolves. A write made in `onFlush` happens after that flush, so
 * it is its own update: it commits on its own unless something it changes
 * also derives from the held work (a memo or `createEffect` reading both),
 * in which case it joins that hold as any later write would.
 *
 * Call it from event handlers and effect callbacks (including
 * `createTrackedEffect` and `onSettled` bodies). In an owned scope such as a
 * component body or a computation it warns in dev: use `onSettled` there.
 *
 * - A write in the callback lands after any write made inline in the same
 *   frame. Don't write the same signal both inside and outside it; use a
 *   functional setter if the order matters.
 * - Several `onFlush` callbacks run in call order, each in its own flush:
 *   async work one callback starts does not hold another's writes.
 * - `queueMicrotask` is not a substitute: called before the frame's first
 *   write, it runs before that flush and its write joins it.
 *
 * @example
 * ```tsx
 * // A menu closes now even when the consumer's onSelect starts async work.
 * const select = (id: string) => {
 *   onFlush(() => setOpen(false));
 *   props.onSelect?.(id);
 * };
 * ```
 *
 * @param callback Function to run once after the pending flush
 */
function onFlush(callback) {
  {
    const owner = getOwner();
    if (owner && !(owner._config & CONFIG_CHILDREN_FORBIDDEN) && !inRefCallback()) {
      const message =
        "[ON_FLUSH_IN_OWNED_SCOPE] onFlush called in an owned scope (a component body or a computation). It is meant for event handlers and effect callbacks; to run work after this scope's first stable render, use onSettled.";
      reportDiagnostic(
        emitDiagnostic({
          code: "ON_FLUSH_IN_OWNED_SCOPE",
          kind: "lifecycle",
          severity: "warn",
          message,
          ownerId: owner.id,
          ownerName: owner._name
        })
      );
    }
  }
  // The frame's flush is queued (or already draining) ahead of this
  // microtask, so the callback's writes start a flush of their own. Each
  // callback flushes before the next runs, so async work one starts never
  // holds another's writes.
  schedule();
  queueMicrotask(() => {
    callback();
    flush();
  });
}

function mapArray(list, map, options) {
  const keyFn = typeof options?.keyed === "function" ? options.keyed : undefined;
  const indexes = map.length > 1;
  const wrappedMap = options?.name
    ? (...args) => {
        setStrictRead(options.name);
        try {
          return map(...args);
        } finally {
          setStrictRead(false);
        }
      }
    : map;
  const data = {
    _owner: createOwner(),
    _len: 0,
    _list: list,
    _items: [],
    _map: wrappedMap,
    _mappings: [],
    _nodes: [],
    _key: keyFn,
    _rows: keyFn || options?.keyed === false ? [] : undefined,
    _indexes: indexes && options?.keyed !== false ? [] : undefined,
    _byIndex: options?.keyed === false,
    _fallback: options?.fallback
  };
  const node = computed(
    updateKeyedMap.bind(data),
    options?.name ? { name: options.name } : undefined
  );
  node._config &= ~CONFIG_AUTO_DISPOSE;
  return accessor(node);
}
const pureOptions = { ownedWrite: true };
function trySmallMove(data, newItems, newLen, start) {
  const oldItems = data._items;
  const oldEnd = data._len - 1;
  const srcPos = [];
  const dstPos = [];
  const runs = []; // flat triples: oldStart, newStart, length
  let budget = 256;
  let i = start;
  let j = start;
  while (i <= oldEnd && j <= newLen - 1) {
    const oldItem = oldItems[i];
    const newItem = newItems[j];
    if (oldItem === newItem) {
      const from = i;
      do j++;
      while (++i <= oldEnd && j < newLen && oldItems[i] === newItems[j]);
      runs.push(from, j - i + from, i - from);
      continue;
    }
    // Bounded realignment lookahead, shorter distance wins.
    let del = -1;
    let lim = Math.min(32 - srcPos.length, oldEnd - i, budget);
    for (let a = 1; a <= lim; a++) {
      if (oldItems[i + a] === newItem) {
        del = a;
        break;
      }
    }
    budget -= del === -1 ? lim : del;
    let ins = -1;
    lim = Math.min(32 - dstPos.length, newLen - 1 - j, budget);
    for (let a = 1; a <= lim; a++) {
      if (newItems[j + a] === oldItem) {
        ins = a;
        break;
      }
    }
    budget -= ins === -1 ? lim : ins;
    if (del !== -1 && (ins === -1 || del <= ins)) {
      while (del-- > 0) srcPos.push(i++);
      continue;
    }
    if (ins !== -1) {
      while (ins-- > 0) dstPos.push(j++);
      continue;
    }
    if (budget <= 0 || srcPos.length === 32 || dstPos.length === 32) return false;
    srcPos.push(i++);
    dstPos.push(j++);
  }
  for (; i <= oldEnd; i++) {
    if (srcPos.length === 32) return false;
    srcPos.push(i);
  }
  for (; j <= newLen - 1; j++) {
    if (dstPos.length === 32) return false;
    dstPos.push(j);
  }
  return commitSmallMove(data, newItems, newLen, start, srcPos, dstPos, runs);
}
/** PHASE 2 of the small-move path, in its OWN function so that a pass which
 * only SCANS and bails (a full replace: the first structural pass of a page,
 * typically) compiles nothing but the scan — V8 parses and compiles lazily
 * per function, and cold `replace` measured +0.5 ms with both phases in one
 * body. Pairs displaced sources with destinations (an unmatched destination
 * is a replacement/insertion → general path), then commits: slice() the live
 * arrays, copy shifted runs, patch displaced pairs, dispose leftovers. */
function commitSmallMove(data, newItems, newLen, start, srcPos, dstPos, runs) {
  const oldItems = data._items;
  let i;
  let j;
  let consumed;
  if (dstPos.length !== 0) {
    consumed = new Array(srcPos.length);
    for (j = 0; j < dstPos.length; j++) {
      let found = -1;
      for (i = 0; i < srcPos.length; i++) {
        if (!consumed[i] && oldItems[srcPos[i]] === newItems[dstPos[j]]) {
          found = i;
          break;
        }
      }
      if (found === -1) return false;
      consumed[found] = true;
      dstPos[j] = (dstPos[j] << 6) | found; // pack pairing (found < 32)
    }
  }
  // DUPLICATES: the general path pairs equal identities by OCCURRENCE ORDER
  // (the chained index map). Displaced↔displaced pairing above is ascending
  // on both sides, so it agrees; but an aligned run was matched by POSITION,
  // and if a displaced identity also occurs inside a run the two algorithms
  // can hand different occurrences different owners (row-local state moves;
  // a shrink could dispose the wrong one). Decline that case — general path.
  // Every displaced identity is a `srcPos` item; any other one in the window sits in a run.
  for (i = 0; i < srcPos.length; i++) {
    const item = oldItems[srcPos[i]];
    // `indexOf` skips holes, which the scan above read as `undefined` rows.
    if (item === undefined) return false;
    for (j = start - 1; (j = oldItems.indexOf(item, j + 1)) !== -1;)
      if (!srcPos.includes(j)) return false;
  }
  const oldMappings = data._mappings;
  const oldNodes = data._nodes;
  const mappings = oldMappings.slice(0, newLen);
  const nodes = oldNodes.slice(0, newLen);
  for (let r = 0; r < runs.length; r += 3) {
    const ro = runs[r];
    const rn = runs[r + 1];
    if (ro !== rn) {
      for (let a = 0; a < runs[r + 2]; a++) {
        mappings[rn + a] = oldMappings[ro + a];
        nodes[rn + a] = oldNodes[ro + a];
      }
    }
  }
  for (j = 0; j < dstPos.length; j++) {
    const p = dstPos[j] >> 6;
    const q = srcPos[dstPos[j] & 63];
    mappings[p] = oldMappings[q];
    nodes[p] = oldNodes[q];
  }
  data._mappings = mappings;
  data._nodes = nodes;
  data._len = newLen;
  data._items = newItems;
  // Dispose unmatched sources LAST (general-path ordering).
  for (i = 0; i < srcPos.length; i++) {
    if (consumed === undefined || !consumed[i]) oldNodes[srcPos[i]].dispose();
  }
  return true;
}
/** Stage a PURE REMOVAL without the key map: when the new window is the old
 * window with items dropped, in order (one-item removals, every-nth, the
 * table filters), a two-pointer walk lands every survivor in the window
 * stage with one key call per item and returns the leavers; a shape that is
 * not an ordered subsequence returns null with the node stage cleared, and
 * the map path below starts from scratch (it rewrites every window slot of
 * `temp` / `rows` / `indexes`, so only `tempNodes`' `!== undefined` probe
 * needs the reset). The stage arrays are scratch — nothing live is touched. */
function stageRemoval(data, newItems, start, end, newEnd, temp, tempNodes, rows, indexes) {
  const key = data._key;
  const oldItems = data._items;
  const nodes = data._nodes;
  const removed = [];
  let i = start;
  for (let j = start; j <= newEnd; i++, j++) {
    const nk = key ? key(newItems[j]) : newItems[j];
    while (i <= end && (key ? key(oldItems[i]) : oldItems[i]) !== nk) removed.push(nodes[i++]);
    if (i > end) {
      tempNodes.fill(undefined);
      return null;
    }
    temp[j] = data._mappings[i];
    tempNodes[j - start] = nodes[i];
    if (rows) rows[j - start] = data._rows[i];
    if (indexes) indexes[j - start] = data._indexes[i];
  }
  for (; i <= end; i++) removed.push(nodes[i]);
  return removed;
}
/** Land a staged window into the live bookkeeping arrays in place: shift the
 * suffix by `dif` (a loop walked from the side that never overwrites an
 * unread slot), take the window from the stage, write survivors' row and
 * index signals. Its own function so the general path's body stays under
 * the JIT's optimization budget — the inlined commit measured ~10% slower on
 * full-window reorders (the small-move note has the same finding). */
function commitWindow(data, newItems, newLen, start, end, newEnd, tempNodes, rows, indexes) {
  const nodes = data._nodes;
  const liveRows = data._rows;
  const liveIndexes = data._indexes;
  const oldLen = data._len;
  const dif = newLen - oldLen;
  let i, j;
  if (dif > 0) {
    // Grow first: a store past the end from the top is a third slower than
    // filling a pre-sized tail (measured on 1k refs, 2026-10-15).
    nodes.length = newLen;
    if (liveRows) liveRows.length = newLen;
    if (liveIndexes) liveIndexes.length = newLen;
    for (i = oldLen - 1, j = newLen - 1; i > end; i--, j--) {
      nodes[j] = nodes[i];
      if (liveRows) liveRows[j] = liveRows[i];
      if (liveIndexes) liveIndexes[j] = liveIndexes[i];
    }
  } else if (dif < 0) {
    for (i = end + 1, j = newEnd + 1; i < oldLen; i++, j++) {
      nodes[j] = nodes[i];
      if (liveRows) liveRows[j] = liveRows[i];
      if (liveIndexes) liveIndexes[j] = liveIndexes[i];
    }
    nodes.length = newLen;
    if (liveRows) liveRows.length = newLen;
    if (liveIndexes) liveIndexes.length = newLen;
  }
  for (j = start; j <= newEnd; j++) {
    nodes[j] = tempNodes[j - start];
    if (liveRows) writeRow((liveRows[j] = rows[j - start]), newItems[j]);
    if (liveIndexes) setSignal((liveIndexes[j] = indexes[j - start]), j);
  }
  for (j = newEnd + 1; j < newLen; j++) {
    if (liveRows) writeRow(liveRows[j], newItems[j]);
    if (liveIndexes && dif !== 0) setSignal(liveIndexes[j], j);
  }
}
// Checks the row itself, not `_items`: an aborted pass can leave a row written or staged.
const writeRow = (row, v) =>
  (row._value === v && row._pendingValue === NOT_PENDING) || setSignal(row, v);
function updateKeyedMap() {
  // Untracked reads inside the internal owner resolve via _parentComputed:
  // routed through the node, a store-proxy lookup sees pending writes (not a
  // stale _value) and an async read in a row callback registers the node as
  // the flight's reader (pending tracking + post-settle retry) instead of
  // vanishing. Set per pass: the first pass runs inside `computed()`, before
  // the node exists to the caller.
  this._owner._parentComputed = getOwner();
  let newItems = this._list() || [];
  newItems[$TRACK]; // top level tracking (a store array's container node)
  // A shallow store array diffs its backing: its slots are raw records, and
  // a trap per slot read was most of a one-row write on a 1k-row list.
  newItems = shallowView?.(newItems[$TARGET]) ?? newItems;
  const newLen = newItems.length;
  runWithOwner(this._owner, updateKeyedRows.bind(this, newItems, newLen));
  return this._mappings;
}
// Not a per-call closure: V8 drops a dead closure's optimized code at each major GC.
function updateKeyedRows(list, newLen) {
  // The next pass's `_items`, taken first: a store list answers each read of a slot through a trap.
  const newItems = list.slice(0);
  let i,
    j,
    // The stage slot a mapper writes its row/index signal into: `j` on the
    // create-all path, `j - start` on the general path, whose stage arrays
    // are sized to the changed WINDOW, not the list.
    slot,
    rows,
    indexes,
    // Mappers write freshly-created row/index signals into the STAGE
    // arrays (`rows`/`indexes`), never into `this._rows`/`this._indexes`.
    mapper = this._rows
      ? this._byIndex
        ? () => {
            rows[slot] = signal(newItems[j], pureOptions);
            return this._map(accessor(rows[slot]), j);
          }
        : () => {
            rows[slot] = signal(newItems[j], pureOptions);
            indexes && (indexes[slot] = signal(j, pureOptions));
            return this._map(accessor(rows[slot]), indexes ? accessor(indexes[slot]) : undefined);
          }
      : this._indexes
        ? () => {
            const item = newItems[j];
            indexes[slot] = signal(j, pureOptions);
            return this._map(item, accessor(indexes[slot]));
          }
        : () => {
            const item = newItems[j];
            return this._map(item);
          };
  // fast path for empty arrays
  if (newLen === 0) {
    if (this._len !== 0) {
      this._owner.dispose(false);
      this._nodes = [];
      this._items = [];
      this._mappings = [];
      this._len = 0;
      this._rows && (this._rows = []);
      this._indexes && (this._indexes = []);
    }
    if (this._fallback && !this._mappings.length) {
      // an aborted fallback attempt leaves an owner without a mapping;
      // dispose it before re-creating
      this._nodes[0]?.dispose();
      this._mappings[0] = runWithOwner((this._nodes[0] = createOwner()), this._fallback);
    }
  }
  // fast path for new create
  else if (this._len === 0) {
    const mappings = new Array(newLen);
    const nodes = new Array(newLen);
    rows = this._rows && new Array(newLen);
    indexes = this._indexes && new Array(newLen);
    try {
      for (j = 0; j < newLen; j++) {
        slot = j;
        mappings[j] = runWithOwner((nodes[j] = createOwner()), mapper);
      }
    } catch (err) {
      for (i = 0; i <= j; i++) nodes[i]?.dispose();
      throw err;
    }
    // commit
    if (this._nodes[0]) this._nodes[0].dispose(); // previous fallback
    this._mappings = mappings;
    this._nodes = nodes;
    rows && (this._rows = rows);
    indexes && (this._indexes = indexes);
    this._items = newItems;
    this._len = newLen;
  } else {
    let start,
      end,
      newEnd,
      item,
      key,
      newIndices,
      newIndicesNext,
      removed,
      created,
      // Dev (attribution engine installed): the items behind the exited and
      // entered rows, for the list-identity census.
      removedItems,
      createdItems;
    // The pass's write to a per-slot signal (a row accessor in index mode,
    // an index accessor in keyed mode). The list's frame lives in these
    // writes as much as in the computed's result, so under a LANE pass they
    // are the lane's frame too (F1): a plain `setSignal` staged them into
    // the action's transaction and the row readers were served the
    // committed value until the action landed — `<For>` without `keyed`
    // showed the pre-action list for the whole action while the keyed modes
    // showed the optimistic one. The engine publishes the write as a
    // derived override on the slot (lanes stage, A17) and lands or
    // supersedes it when a plain pass later writes a slot still carrying
    // one (A18) — `landOnOverride`'s slot arm. Decided once per pass, not
    // per slot: a lane pass marks the map (`_laneSlots`: its slots may
    // carry overrides), a marked map routes every slot write through the
    // engine while any lane is live — a slot's override resolves with its
    // lane's transaction, or with the batch for an orphan lane
    // (resolveOptimisticNodes, then cleanupCompletedLanes, both before the
    // completion wakes any recompute), so a pass that finds no lane live
    // knows the slots are clean and drops the mark — and an unmarked map
    // takes `writeRow` directly. The engine is installed whenever a lane
    // is active.
    // CARVE 2: the lane-slot arm (`_landOnOverride`, F1) went with the engine.
    // skip common prefix
    for (
      start = 0, end = Math.min(this._len, newLen);
      start < end &&
      (this._items[start] === newItems[start] ||
        (this._rows && compare(this._key, this._items[start], newItems[start])));
      start++
    ) {
      if (this._rows) writeRow(this._rows[start], newItems[start]);
    }
    // skip common suffix — counted only; retained entries land in one pass
    // at commit instead of being staged and copied twice
    for (
      end = this._len - 1, newEnd = newLen - 1;
      end >= start &&
      newEnd >= start &&
      (this._items[end] === newItems[newEnd] ||
        (this._rows && compare(this._key, this._items[end], newItems[newEnd])));
      end--, newEnd--
    );
    // no structural change (every position matched in place at equal
    // length — the common post-reconcile shape): keep the same mapped
    // array identity so downstream consumers don't re-run at all
    if (start === newLen && this._len === newLen) {
      this._items = newItems;
      return;
    }
    // SMALL-MOVE FAST PATH: extracted to its own function — inlining it
    // here bloats updateKeyedMap past the JIT's optimization budget and
    // deoptimizes the GENERAL path (measured 2x on reverse). Gated to
    // LARGE trimmed windows: when the trims already shrank the window
    // (plain removals, tail edits), the general path is window-
    // proportional and cheap — the fast path would only re-walk what the
    // trims proved.
    if (
      newLen <= this._len &&
      end - start > 64 &&
      this._rows === undefined &&
      this._indexes === undefined
    ) {
      // PROBE before the scan: a small move keeps a mid-window item within
      // ±32 of its old position; a REPLACE (all fresh items — the shape
      // every page's first structural pass usually is) has it nowhere.
      // ~65 compares, no allocation, and the scan function is never
      // compiled for a replace (its cold first-call compile was the cost).
      const m = start + ((newEnd - start) >> 1);
      const probe = newItems[m];
      const hi = Math.min(end, m + 32);
      let k = Math.max(start, m - 32);
      while (k <= hi && this._items[k] !== probe) k++;
      if (k <= hi && trySmallMove(this, newItems, newLen, start)) return;
    }
    const dif = newLen - this._len;
    // WINDOW-SIZED STAGING, IN-PLACE COMMIT (2026-10-15). The strong-abort
    // contract (#2903) only needs the pass to touch nothing live before
    // every mapper has succeeded; it never needed four fresh newLen-sized
    // arrays. Only the changed window [start, newEnd] is staged — the
    // prefix and suffix are, by construction, the rows that did not move
    // relative to each other — and at commit the bookkeeping arrays
    // (`_nodes`, `_rows`, `_indexes`) take the window in place with the
    // suffix shifted by a plain loop (`copyWithin` is a generic
    // per-element builtin in V8 — 30× slower than the loop on 1k refs,
    // measured 2026-10-15). `_mappings` alone is
    // rebuilt: its fresh identity is the change signal downstream relies
    // on. A one-row edit on a 500-row list used to allocate ~20 KB of
    // staging plus a position-sized chain array per commit (uibench's
    // one-item tree ops at 4–6× Octane, jfb-reorder `removefirst` at
    // 2.5×); now it stages one slot.
    const wLen = newEnd - start + 1;
    const temp = new Array(newLen);
    const tempNodes = new Array(wLen);
    rows = this._rows ? new Array(wLen) : undefined;
    indexes = this._indexes ? new Array(wLen) : undefined;
    // A shrinking pass is tried as a pure removal first (stageRemoval: no
    // key map, one key call per item); anything else takes the map path.
    const pure =
      dif < 0
        ? stageRemoval(this, newItems, start, end, newEnd, temp, tempNodes, rows, indexes)
        : null;
    if (pure !== null) {
      // (The list-churn census below wants removals AND creations in one
      // pass; a pure removal has none, so no item ledger is kept here.)
      if (pure.length !== 0) removed = pure;
    } else {
      // 0) prepare a map of all indices in the changed window of newItems,
      // scanning backwards so we encounter them in natural order
      newIndices = new Map();
      newIndicesNext = new Array(wLen);
      for (j = newEnd; j >= start; j--) {
        item = newItems[j];
        key = this._key ? this._key(item) : item;
        i = newIndices.get(key);
        newIndicesNext[j - start] = i === undefined ? -1 : i;
        newIndices.set(key, j);
      }
      // 1) step through the old changed window and see if items can be found
      // in the new set; if so, stage them at their new positions; if not,
      // queue them for disposal at commit
      for (i = start; i <= end; i++) {
        item = this._items[i];
        key = this._key ? this._key(item) : item;
        j = newIndices.get(key);
        if (j !== undefined && j !== -1) {
          temp[j] = this._mappings[i];
          tempNodes[j - start] = this._nodes[i];
          rows && (rows[j - start] = this._rows[i]);
          indexes && (indexes[j - start] = this._indexes[i]);
          j = newIndicesNext[j - start];
          newIndices.set(key, j);
        } else {
          (removed ??= []).push(this._nodes[i]);
          if (attrHooks !== null) (removedItems ??= []).push(item);
        }
      }
      // 2) create new rows into the window stage; an abort disposes only these
      try {
        for (j = start; j <= newEnd; j++) {
          if (tempNodes[j - start] !== undefined) continue;
          (created ??= []).push((tempNodes[j - start] = createOwner()));
          if (true && attrHooks !== null) (createdItems ??= []).push(newItems[j]);
          slot = j - start;
          temp[j] = runWithOwner(tempNodes[j - start], mapper);
        }
      } catch (err) {
        if (created) for (i = 0; i < created.length; i++) created[i].dispose();
        throw err;
      }
    }
    // 3) commit: the retained prefix and suffix land in the fresh mapped
    // array; the bookkeeping arrays take the staged window IN PLACE (own
    // function: kept out of this body on purpose, see commitWindow); then
    // the exited rows dispose
    for (i = 0; i < start; i++) temp[i] = this._mappings[i];
    for (j = newEnd + 1; j < newLen; j++) temp[j] = this._mappings[j - dif];
    commitWindow(this, newItems, newLen, start, end, newEnd, tempNodes, rows, indexes);
    this._mappings = temp;
    this._len = newLen;
    this._items = newItems;
    if (removed) for (i = 0; i < removed.length; i++) removed[i].dispose();
    if (attrHooks !== null && removedItems !== undefined && createdItems !== undefined)
      attrHooks.listChurn(
        this._owner._parentComputed,
        removedItems,
        createdItems,
        newLen,
        this._key !== undefined
      );
  }
}
/**
 * Reactively renders a callback `count` times, reusing previously-rendered
 * entries when only the count changes. Underlying helper for `<Repeat>`.
 *
 * - `options.from` — start index (default `0`); useful for offset/windowed
 *   rendering.
 * - `options.fallback` — accessor returning a value to show when count is `0`.
 *
 * @example
 * ```ts
 * const view = repeat(count, i => `Item ${i}`, { fallback: () => "empty" });
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/advanced/jsx-component-primitives/repeat
 */
function repeat(count, map, options) {
  const wrappedMap = options?.name
    ? i => {
        setStrictRead(options.name);
        try {
          return map(i);
        } finally {
          setStrictRead(false);
        }
      }
    : map;
  const data = {
    _owner: createOwner(),
    _len: 0,
    _offset: 0,
    _count: count,
    _map: wrappedMap,
    _nodes: [],
    _mappings: [],
    _from: options?.from,
    _fallback: options?.fallback
  };
  const node = computed(updateRepeat.bind(data));
  node._config &= ~CONFIG_AUTO_DISPOSE;
  return accessor(node);
}
// Same staged-commit discipline as `updateKeyedMap` (#2903): the retained
// window overlap is copied into fresh arrays, missing indexes are created
// into them, and `this` is only touched — including disposal of rows leaving
// the window — after every `_map` call succeeded. A NotReadyError mid-pass
// disposes only the owners this pass created and leaves prior state intact
// for the post-settle retry. The overlap math also subsumes the previous
// disjoint-window/front-clear/end-clear/shift special cases.
function updateRepeat() {
  // As in updateKeyedMap: the node is the row callbacks' reader, per pass.
  this._owner._parentComputed = getOwner();
  const newLen = this._count();
  const from = this._from?.() || 0;
  runWithOwner(this._owner, updateRepeatRows.bind(this, newLen, from));
  return this._mappings;
}
// Not a per-call closure: V8 drops a dead closure's optimized code at each major GC.
function updateRepeatRows(newLen, from) {
  if (newLen === 0) {
    if (this._len !== 0) {
      this._owner.dispose(false);
      this._nodes = [];
      this._mappings = [];
      this._len = 0;
      // Reset offset to match the cleared data (#2767, repro 2).
      this._offset = 0;
    }
    if (this._fallback && !this._mappings.length) {
      // an aborted fallback attempt leaves an owner without a mapping;
      // dispose it before re-creating
      this._nodes[0]?.dispose();
      this._mappings[0] = runWithOwner((this._nodes[0] = createOwner()), this._fallback);
    }
    return;
  }
  const to = from + newLen;
  const prevTo = this._offset + this._len;
  // Retained overlap [keepStart, keepEnd) in global indexes; empty when the
  // windows are disjoint or when coming from empty/fallback.
  const keepStart = Math.max(from, this._offset);
  const keepEnd = Math.min(to, prevTo);
  const mappings = new Array(newLen);
  const nodes = new Array(newLen);
  for (let i = keepStart; i < keepEnd; i++) {
    nodes[i - from] = this._nodes[i - this._offset];
    mappings[i - from] = this._mappings[i - this._offset];
  }
  try {
    for (let i = from; i < to; i++) {
      if (i >= keepStart && i < keepEnd) continue;
      mappings[i - from] = runWithOwner((nodes[i - from] = createOwner()), () => this._map(i));
    }
  } catch (err) {
    for (let i = from; i < to; i++)
      if ((i < keepStart || i >= keepEnd) && nodes[i - from]) nodes[i - from].dispose();
    throw err;
  }
  // commit: dispose the previous fallback or the rows leaving the window
  if (this._len === 0) this._nodes[0]?.dispose();
  else
    for (let i = this._offset; i < prevTo; i++)
      if (i < from || i >= to) this._nodes[i - this._offset].dispose();
  this._mappings = mappings;
  this._nodes = nodes;
  this._offset = from;
  this._len = newLen;
}
function compare(key, a, b) {
  return key ? key(a) === key(b) : true;
}

// Set before any family is optimistic (`fam.opt`): a call site checks the
// hooks first, so a build that does not import optimistic.ts folds the arm
// away.
let optHooks = null;
function installOptHooks(hooks) {
  optHooks = hooks;
}
let affectsHooks = null;
function installAffectsHooks(hooks) {
  affectsHooks = hooks;
}

/**
 * Store — plain stores on L2 (plan sec. 31, S1).
 *
 * A store is a tree of L2 nodes and nothing else carries reactive state:
 *
 * - **Leaf nodes** (one per tracked-or-written key, lazy; `slotSignal`, one
 *   literal): `_value` committed, `_pendingValue` this batch's staging.
 * - **Presence nodes** (`in` tracks presence, not value — R13) and the
 *   **deep witness** (`deep()` subscribes one node per record).
 * - The **container node** `k` (plan sec. 31.3, Q-A): the `$TRACK` node given a
 *   value. `_value` is the committed backing, `_pendingValue` the pending
 *   backing the draft mutates — so a write to a key nobody reads has a
 *   staging home the scheduler owns, and park/commit/revert of the backing
 *   is the hold model's, not a fold ledger of the store's. Structural
 *   readers (`ownKeys`, iteration, `$TRACK`, `length`, `deep`) subscribe to
 *   it; value readers never do — a key with no leaf reads through the
 *   container's visible backing without subscribing (R1: node existence is
 *   unobservable).
 *
 * The committed backing (`t.v`) is owned raw (CoW: a user object is shared
 * until the first write privatizes it and its ancestor chain). It mutates at
 * exactly one moment — the flush's commit (`_storeCommit`), after the
 * container node committed its staging. Reads: context-free readers see the
 * committed backing until the flush; a pass sees the staging (signal
 * parity, RUL-1); the draft reads its own staging.
 *
 * Perf contracts (plan sec. 31.4): lazy nodes; one literal per node; O(written) per
 * setter (`wk`); overlay drafts for large plain records; zero allocation
 * for adopted-but-unread objects.
 */
/** Key of the deep-witness slot node (never a user key). */
const $DEEP = Symbol("STORE_DEEP");
// ---------------------------------------------------------------------------
// wrap / dedupe
/** Pre-shaped constructor for store targets: V8 tips a bare `{}` into
 * dictionary mode once ~19 named properties are assigned onto it (the #3044
 * `ovl`/`del` fields crossed that line — every trap's field read became a
 * hash lookup, a 15% deep-dbmon tick regression). Declaring every field in a
 * constructor pre-allocates in-object slots so the map stays fast, with
 * headroom for future fields. It is the proxy's handler: the traps live on
 * its prototype and read it as `this`. */
function TargetShape(value, parent, parentKey, fam) {
  // Direct field assignment in one fixed order (no Object.assign literal
  // copy): every target shares a hidden-class transition chain — createTarget
  // was the #2 store cost in the uibench creation profile.
  this.v = value;
  // Chained-backing flag (backing IS another store's proxy, §7b) — cached so
  // the hot read path never does a per-read symbol lookup on the backing.
  this.ch = value[$TARGET] !== undefined;
  this.pb = null;
  this.n = null;
  this.h = null;
  this.k = null;
  this.dk = null;
  this.u = parent;
  this.pk = parentKey;
  this.px = null;
  this.d = false;
  this.a = false;
  this.sc = 0;
  this.kc = 0;
  this.nc = 0;
  this._fam = fam;
  this.o = fam !== null && fam.opt === true;
  this.s = false;
  this.ovl = false;
  this.del = null;
  this.wk = null;
}
function createTarget(value, parent, parentKey, fam = parent?._fam ?? null) {
  const t = new TargetShape(value, parent, parentKey, fam);
  // Empty: V8 checks trap results against the proxy target's own keys.
  t.px = new Proxy(Array.isArray(value) ? [] : {}, t);
  setLookup(value, t);
  return t;
}
function wrap(value, parent = null, parentKey = null, fam = parent?._fam ?? null) {
  // markRaw'd values never wrap through ANY store (R42; sticky raw-marking
  // is one half of the never-both-wrapped-and-raw invariant, RUL-12).
  if (rawValuesUsed && isRawValue(value)) return value;
  const existing = lookupTarget(value, fam);
  if (existing !== undefined) return existing.px;
  const t = value[$TARGET];
  if (t?.px === value) {
    // Foreign-family proxies re-wrap into THIS family (writes stay isolated);
    // same-family and plain-store proxies pass through.
    if (fam === null || t._fam === fam) return value;
    return createTarget(value, parent, parentKey, fam).px;
  }
  return createTarget(value, parent, parentKey, fam).px;
}
/** Unwrap our own proxies to their current backing; leave everything else.
 * A projection draft wrapper is registered as its target's alias
 * (projection.ts `wrapDraft`) — adopted as a backing, it would forward to
 * itself (#3767). */
function unwrapValue(v) {
  if (v == null || typeof v !== "object") return v;
  const t = v[$TARGET];
  if (t !== undefined && (t.px === v || lookupTarget(v, t._fam) === t) && t.v !== undefined) {
    // A draft escaping into other storage must be a REAL container that
    // becomes this target's committed backing at fold (the shared-raw
    // contract) — a prototype overlay is neither.
    if (t.ovl) materializePB(t);
    return t.pb ?? t.v;
  }
  return v;
}
// ---------------------------------------------------------------------------
// nodes: pure subscription points (values used only for equality gating)
// Shared slot-node equality (create-floor diet): ONE function for every
// leaf — `this` is the node (method-call convention at every _equals site),
// `_host` is the baked-in target backref. Logical-slot equality: values
// resolving to the same child target are the same slot (privatization/
// adoption swap raw identity without changing the logical value — only
// changed leaves notify, R9).
const slotNodeEquals = function (a, b) {
  return isEqual(a, b) || sameLogicalSlot(this._host, a, b);
};
/** The deep witness: the store decides when it notifies. */
const never = false;
function sameKey(a, b) {
  return a === b || (a !== a && b !== b);
}
/** The container node's comparator: a staging is a backing (identity — a
 * pending backing is never "equal" to the committed one). A guess never
 * sits on the container: it is the target's carrier's (optimistic.ts, S2). */
const containerEquals = function (_a, _b) {
  return _a === _b;
};
function sameLogicalSlot(target, a, b) {
  if (a === null || typeof a !== "object" || b === null || typeof b !== "object") return false;
  const at = lookupTarget(a, target._fam);
  return at !== undefined && at === lookupTarget(b, target._fam);
}
/** Slot nodes whose last subscriber left while they still held a staging:
 * released at the commit that resolves it (`drainFolds`). */
const deferredReleases = new Set();
function releaseSlot(node) {
  const t = node._host;
  const key = node._key;
  if (key === $TRACK) {
    if (t.k === node) t.k = null;
  } else if (key === $PATCH) {
    return; // a guess carrier lives and dies with its lane (optimistic.ts sweep)
  } else if (key === $DEEP) {
    if (t.dk === node) t.dk = null;
  } else if (t.n?.[key] === node) {
    delete t.n[key];
    // An emptied dictionary is dropped (#5024): the target outlives its
    // readers while the app holds the raw object.
    if (--t.nc === 0) t.n = null;
  } else if (t.h?.[key] === node) {
    delete t.h[key];
    if (Reflect.ownKeys(t.h).length === 0) t.h = null;
  }
  // The family's index keeps only targets that still carry a node.
  if (t._fam !== null && t.nc === 0 && t.k === null && t.dk === null && t.h === null)
    t._fam.live.delete(t);
}
/** A node was born on `target`: a family target enters the live index. */
function noteNode(target, node, key) {
  if (target._fam !== null) target._fam.live.add(target);
  markDescendants(target);
  if (affectsHooks !== null) affectsHooks._born(target, node, key);
}
// Shared slot-node release handler: registered once; the core sweep
// dispatches CONFIG_SLOT_NODE nodes here (graph.ts) instead of holding a
// per-node closure in a per-node NodeExtension. A dropped node is
// unreachable through the store — a fresh read makes a fresh node.
// A staged write — or a lane's value (a guess, S4) — is state only the node
// holds: its release waits for the commit that resolves it. So does a leaf of
// a held container (#4146): a mainline write it carried is in the held
// staging too, where it would read as the hold's until the landing.
const slotKept = node =>
  node._pendingValue !== NOT_PENDING ||
  node._config & CONFIG_OVERRIDE ||
  node._host.k?._config & CONFIG_HELD;
setSlotUnobserved(node => {
  if (slotKept(node)) deferredReleases.add(node);
  else releaseSlot(node);
});
/** A node born while its container has a staging is born from the two
 * frames (A29): committed from the committed backing, the staging from the
 * pending one — and held by the container's transaction when the container
 * is held (#3706: the unit of the hold is the key — a key the batch left
 * unchanged stages nothing and holds no one). */
function bornStaged(target, node, staged, key) {
  if (node._equals && node._equals(node._value, staged)) return;
  queuePendingNode(node);
  node._pendingValue = staged;
  const k = target.k;
  if (k === null) return; // a node-less staging holds nothing (`stageOn`)
  // The staging is a derive's lane pass's (`stageOn` → `holdWithDerive`
  // listed the container in the lane, #4478), not a guess's: the leaf takes
  // it as the lane's derivation, as the write would have left a leaf that
  // existed then — a lane reader sees the lane's value, the frame the
  // committed one, as a reader of the memo the derive reads does (E3,
  // D-25). Born a staging instead, the park held it with the frame, and an
  // Errored the memo's lane value heals stayed on its fallback over the
  // store (#4451, D-39).
  if (laneStageHook && (k._config & (CONFIG_OVERRIDE | CONFIG_GUESS)) === CONFIG_OVERRIDE)
    holdWithDerive(target, node, txOf(k));
  // Held by the container's transaction only for a key the HELD staging
  // changed — a mainline layer's change above it stages mainline. (A
  // container carrying a guess is a lane's: the flush's park decides —
  // unless a correction folded under the guess, D-34: the key is that
  // correction's hold's.)
  else if (k._config & CONFIG_HELD && heldKeyChanged(target, key)) {
    const l = txOf(k);
    if (!(k._config & CONFIG_OVERRIDE) || l.folded) holdNode(node, resolveTx(l._parent ?? l));
  }
  // A28 (4): a batch written inside a creation-time pass is promoted — its
  // late-materialized leaf with it.
  if (k._config & CONFIG_PROMOTED) notePromotedWrite(node);
}
function getNode(
  target,
  key,
  // First-read dedupe (create-floor slice 2): the get trap probes
  // accessor-ness right before creating the node — pass the verdict through
  // so creation skips the second descriptor scan. -1 = unknown (other
  // callers), 0/1 = probed.
  accKnown = -1
) {
  const nodes = (target.n ??= Object.create(null));
  let node = nodes[key];
  if (node === undefined) {
    // Accessor-ness resolved ONCE per node (no per-object descriptor scan
    // on reads): accessor keys serve through Reflect.get with the proxy
    // receiver.
    const pb = target.pb;
    const created = (node = slotSignal(
      committed(target)[key],
      slotNodeEquals,
      target,
      key,
      accKnown === -1 ? isAccessorKey(pb ?? target.v, key) : accKnown === 1
    ));
    if (pb !== null && !created.acc)
      bornStaged(target, created, target.del?.has(key) ? undefined : pb[key], key);
    // Attribution-only: name store property nodes by path segment so
    // attribution chains and wide-scope warnings read "store.todos" (or
    // "todos.title" when the store was declared with a name), not
    // "signal". Gated on the engine being installed — node creation is
    // the hottest store path, and the disabled cost must stay one null
    // check (nodes created before enable() stay generically named).
    if (attrHooks !== null) {
      const root = storeRoot(target);
      created._name = (storeNames.get(root) ?? "store") + "." + String(key);
      created._loc = storeLocs.get(root);
      stampNodeOwner(created, target);
    }
    nodes[key] = node;
    target.nc++;
    noteNode(target, created, key);
  }
  return node;
}
function getHasNode(target, key) {
  const nodes = (target.h ??= Object.create(null));
  let node = nodes[key];
  if (node === undefined) {
    const created = (node = slotSignal(key in committed(target), isEqual, target, key, false));
    const pb = target.pb;
    if (pb !== null) bornStaged(target, created, key in pb && !target.del?.has(key), key);
    if (attrHooks !== null) stampNodeOwner(created, target);
    nodes[key] = node;
    noteNode(target, created, key);
  }
  return node;
}
function nameCarrier(g, t) {
  if (attrHooks !== null) {
    const root = storeRoot(t);
    g._name = (storeNames.get(root) ?? "store") + ".$guess";
    g._loc = storeLocs.get(root);
    stampNodeOwner(g, t);
  }
}
/** The container node (plan sec. 31.3): created by the first structural subscription
 * or the first write that needs to stage. Its committed value is the
 * committed backing. */
function getContainerNode(target) {
  let k = target.k;
  if (k === null) {
    k = target.k = slotSignal(target.v, containerEquals, target, $TRACK, false);
    if (attrHooks !== null) stampNodeOwner(k, target);
    noteNode(target, k, $TRACK);
    // Born while a batch is staged without a node (`stageOn`): the node
    // takes the staging as its own — the pre-batch backing committed, the
    // pending backing staged — so whoever asked for it reads the frame it
    // would have read had the node been there from the write.
    if (target.pb !== null && !directCommit) {
      k._value = preBatch(target);
      k._pendingValue = target.pb;
      queuePendingNode(k);
    }
  }
  return k;
}
/** Q-A's escape hatch (plan sec. 33; measured plan sec. 41.4): a staging needs the container
 * node as its home only when something can read the frame through it — a
 * structural subscriber (the node exists), lane work (the staging is the
 * lane's), a pass outside a flush (its write is promoted at the pass's end,
 * A28 (4)), or a derive a transaction holds (the staging is the derive's,
 * `holdWithDerive`). Otherwise the backing and the fold queue are the whole
 * staging: nothing allocates, nothing is swept at the commit (dbmon's tick
 * re-created, queued, swept and dropped ~7000 container nodes). If the
 * flush parks, the seam materializes and holds the node then
 * (`storeParkHook`) — the one case the staging home exists for. */
function stageOn(target, pb) {
  if (
    target.k === null &&
    passLane === null &&
    (context === null || globalQueue._running) &&
    !deriveHolds(target)
  )
    return;
  const k = getContainerNode(target);
  if (k._pendingValue === NOT_PENDING) queuePendingNode(k);
  k._pendingValue = pb;
  // A28 (4): a write issued inside a pass outside a flush (a creation-time
  // derive) is promoted at the pass's end — the batch with it.
  if (context !== null) notePromotedWrite(k);
  if (target._fam !== null && !userWrite) holdWithDerive(target, k);
}
/** The projection's derive is held by a transaction and this is its own
 * write (not a user setter's): the staging is the derive's (`holdWithDerive`). */
function deriveHolds(target) {
  const fw = target._fam?.node;
  return fw != null && !userWrite && (fw._config & CONFIG_HELD) !== 0;
}
const storePark = t => {
  for (let i = 0; i < foldList.length; i++) {
    const target = foldList[i];
    if (target.k === null && target.pb !== null) holdNode(getContainerNode(target), t);
  }
  optHooks?.park(t);
};
// Set where the only deep node is created, so a bundle without deep() folds its guards away.
let deepUsed = false;
function getDeepNode(target) {
  let dk = target.dk;
  if (dk === null) {
    deepUsed = true;
    dk = target.dk = slotSignal(0, never, target, $DEEP, false);
    if (attrHooks !== null) stampNodeOwner(dk, target);
    noteNode(target, dk, $DEEP);
  }
  return dk;
}
function bumpDeep(t) {
  if (t.dk !== null) setSignal(t.dk, 1);
}
/** The structural readers' one notification: a membership/arrangement
 * change on the container (the staging is already on the node; `setSignal`
 * re-stages the same backing and walks the subscribers — and, on later
 * steps, routes a held or lane-owned container the way any node is). */
function notifyContainer(k, pb) {
  // A container carrying an arrangement guess takes the write as a landing
  // on the guess (`laneWrite` → `supersede`, judged by `containerEquals`);
  // otherwise the structural readers are walked (the staging is already on
  // the node; the comparator is the store's, done).
  // Not a derive's lane pass that listed the container already (`stageOn` →
  // `holdWithDerive`, #4478): there is no guess to land on, the staging is
  // the lane's derivation, and its structural readers are walked like any
  // (their reads of it are the lane's, `laneRead`). Taken as a landing, the
  // write equalled the staging it already was and nobody heard of it: a
  // `mapArray` over the projection kept the old arrangement for good (#4843).
  if ((k._config & (CONFIG_OVERRIDE | CONFIG_GUESS)) === (CONFIG_OVERRIDE | CONFIG_GUESS))
    setSignal(k, pb);
  else {
    if (k._config & CONFIG_HELD) joinFuture(txOf(k));
    if (snapshotCaptureActive) captureWriteSnapshot(k, k._value);
    insertSubs(k);
    schedule();
  }
}
// Observe-tier labels and owners (attribution).
function storeRoot(target) {
  let root = target;
  while (root.u !== null) root = root.u;
  return root;
}
function storeRootOwner(target) {
  return storeOwners.get(storeRoot(target));
}
function stampNodeOwner(created, target) {
  created._owner = storeRootOwner(target) ?? null;
}
const storeNames = new WeakMap();
// The store's `loc` option, carried onto every property node beside its
// label (the same gate: nodes created before the engine is installed carry
// neither).
const storeLocs = new WeakMap();
function nameStore(proxy, name, loc) {
  if (attrHooks !== null) {
    const target = proxy[$TARGET];
    if (name) storeNames.set(target, name);
    if (loc) storeLocs.set(target, loc);
  }
}
// ---------------------------------------------------------------------------
// clones (CoW privatization)
const hasOwn$1 = Object.prototype.hasOwnProperty;
// Aliased: each spelled-out use costs ~20 minified bytes on every store row.
const protoOf = Object.getPrototypeOf;
const setProto = Object.setPrototypeOf;
const lookupGetter = Object.prototype.__lookupGetter__;
const lookupSetter = Object.prototype.__lookupSetter__;
const propertyIsEnumerable$1 = Object.prototype.propertyIsEnumerable;
function isOwnAccessor(src, key) {
  return (
    hasOwn$1.call(src, key) &&
    (lookupGetter.call(src, key) !== undefined || lookupSetter.call(src, key) !== undefined)
  );
}
// Own accessors, and a class's prototype getters (they run on the proxy).
function isAccessorKey(src, key) {
  return (
    isOwnAccessor(src, key) ||
    (key !== "__proto__" && !hasOwn$1.call(src, key) && lookupGetter.call(src, key) !== undefined)
  );
}
/** Shallow clone into store ownership. Plain data (grade 2) spreads; anything
 * else copies descriptors (accessors kept, everything made configurable). */
function cloneRaw(source, t) {
  t.sc || scanAccessorsOnce(t);
  if (t.sc === 2) {
    const clone = { ...source };
    clone[$OWNER] = t;
    return clone;
  }
  // A plain array with no accessors and no named keys beyond `length`
  // (`kc` counts own keys) clones by `slice()`: the descriptor path
  // allocates one descriptor per element, O(n) per first draft write (10k
  // rows: ~10 ms per push).
  let clone;
  if (!t.a && t.kc <= source.length + 1 && protoOf(source) === Array.prototype)
    clone = source.slice();
  else {
    const descs = Object.getOwnPropertyDescriptors(source);
    for (const key of Reflect.ownKeys(descs)) {
      const d = descs[key];
      if (key === "length" && Array.isArray(source)) continue;
      d.configurable = true;
      if (!d.get && !d.set) d.writable = true;
      else t.a = true;
    }
    // An array subclass keeps its prototype (#4670): the copy is a real
    // array (`Array.isArray`, exotic `length`) re-prototyped to the source's.
    clone = Object.defineProperties(
      Array.isArray(source) ? setProto([], protoOf(source)) : Object.create(protoOf(source)),
      descs
    );
  }
  clone[$OWNER] = t;
  return clone;
}
/** Wide plain clone (many keys): null-prototype build, then reset. */
function wideClone(source, t) {
  const clone = Object.create(null);
  for (const key of Reflect.ownKeys(source)) clone[key] = source[key];
  setProto(clone, Object.prototype);
  clone[$OWNER] = t;
  return clone;
}
function copyOwn(to, from, key) {
  const d = Object.getOwnPropertyDescriptor(from, key);
  if (d.get || d.set || !d.enumerable || !d.writable || !d.configurable)
    Object.defineProperty(to, key, d);
  else to[key] = d.value;
}
/** One descriptor scan per container (grade 1/2, `a`, `kc`). */
function scanAccessorsOnce(target) {
  const src = target.v;
  const keys = Reflect.ownKeys(src);
  let plain = protoOf(src) === Object.prototype;
  for (const key of keys) {
    if (isOwnAccessor(src, key)) {
      target.a = true;
      plain = false;
      break;
    }
    if (plain && !propertyIsEnumerable$1.call(src, key)) plain = false;
  }
  target.sc = plain ? 2 : 1;
  target.kc = keys.length;
  return !target.a;
}
// ---------------------------------------------------------------------------
// the pending backing
const OVERLAY_MIN_KEYS = 32;
const OVERLAY_REBUILD_MAX_KEYS = 1024;
const OVERLAY_REBUILD_MIN_ADDS = 16;
const WK_ALL = new Set();
const plainProto = o => {
  const p = protoOf(o);
  return p === Object.prototype || p === Array.prototype || p === null;
};
/** An overlay with many adds (or any delete) rebuilds the committed
 * container at flatten so its shape stays fast (#3689). */
function overlayRebuilds(t, pb) {
  if (t.kc > OVERLAY_REBUILD_MAX_KEYS) return false;
  if (t.del?.size) return true;
  const wk = t.wk;
  if (wk !== null && wk !== WK_ALL && wk.u === undefined && wk.size <= OVERLAY_REBUILD_MIN_ADDS)
    return false;
  const v = t.v;
  let adds = 0;
  for (const key of Reflect.ownKeys(pb))
    if (!hasOwn$1.call(v, key) && ++adds > OVERLAY_REBUILD_MIN_ADDS) return true;
  return false;
}
/** Downgrade an overlay to a real clone (a consumer needs a container). */
function materializePB(target) {
  const pb = target.pb;
  if (!target.ovl) return pb;
  const clone = overlayOnto(
    target,
    target.sc === 2 ? wideClone(target.v, target) : cloneRaw(target.v, target),
    pb
  );
  target.pb = clone;
  // The staging IS the pending backing: re-point the container node.
  if (target.k?._pendingValue === pb) target.k._pendingValue = clone;
  return clone;
}
/** The batch's pending backing, created on the first draft write: an overlay
 * of an owned plain-data record above the key threshold, a clone otherwise.
 * Staged on the container node (the scheduler owns its lifetime) and queued
 * for the fold at the flush's commit. */
function ensurePB(target) {
  let pb = target.pb;
  // An optimistic edit composes on the view the user saw and becomes
  // guesses at the setter's exit: nothing is staged, nothing folds
  // (optimistic.ts).
  if (optHooks !== null && target.o && userWrite && !pendingNotify.has(target))
    return (target.pb = optHooks.draft(target));
  if (pb !== null && !isOwned(pb)) {
    // An adopted object awaiting its fold is the user's: the draft writes a
    // clone of it. The clone is the batch's staging — unless the adoption is
    // HELD (another transaction's): then the held staging stays the
    // adoption and the draft is a mainline layer above it (#3612/#3688: a
    // mainline setter mid-hold publishes mainline; the held keys stay held).
    pb = target.pb = cloneRaw(pb, target);
    const k = target.k;
    if (k !== null && !(k._config & CONFIG_HELD)) k._pendingValue = pb;
  } else if (pb === null) {
    const v = target.v;
    if (!directCommit) queueFold(target);
    if (
      !target.ch &&
      !Array.isArray(v) &&
      (target.sc !== 0 ? !target.a : scanAccessorsOnce(target)) &&
      target.kc > OVERLAY_MIN_KEYS &&
      isOwned(v)
    ) {
      pb = target.pb = Object.create(v);
      target.ovl = true;
    } else
      try {
        pb = target.pb = cloneRaw(v, target);
      } catch (e) {
        // #4808: a nested write through a chained store still loading its
        // first answer (the clone reads it) is dropped, as a write to a
        // loading store is — the first answer replaces it (A25) — and the
        // outer keeps following the inner store.
        if (!(e instanceof NotReadyError)) throw e;
        devWriteThroughLoading(target);
        // The write lands on a backing no one reads.
        return {};
      }
    if (!directCommit) stageOn(target, pb);
  }
  return pb;
}
/** Privatize `target`'s committed backing (and its ancestor chain — a shared
 * parent cannot point at an owned child). Not a reactive event. */
function privatizeCommitted(target) {
  if (isOwned(target.v)) return;
  const before = target.v;
  const clone = cloneRaw(before, target);
  // Mid-batch (a staging is open on this target) the fold still sees the
  // backing the batch started from: the queue recorded it (`foldOlds`).
  target.v = clone;
  target.ch = false;
  if (target.u) {
    privatizeCommitted(target.u);
    const pv = target.u.v;
    const slot = parentSlotKey(target, before);
    if (pv[slot] === before) pv[slot] = target.v;
  }
}
/** The key this target sits under in its parent — re-resolved by identity
 * when an array parent moved it (#3282). Not searched when the slot already
 * holds this target's backing (a parent adopted whole carries its children's
 * new raws before the children fold — the miss was O(rows) per changed row
 * of a keyed reconcile). */
function parentSlotKey(target, expected) {
  const pk = target.pk;
  const pv = target.u.v;
  if (pv[pk] === expected || pv[pk] === target.v || !Array.isArray(pv)) return pk;
  const at = pv.indexOf(expected);
  if (at === -1) return pk;
  target.pk = at;
  return at;
}
/** Overlay flatten (#3044): own keys onto the owned committed backing in
 * place — the backing keeps its identity. */
function flattenOverlay(t, pb) {
  privatizeCommitted(t);
  overlayOnto(t, t.v, pb); // the caller (`foldTarget`) closes the staging
}
/** An overlay's own keys and deletions onto `to` (the two are disjoint: a
 * write un-deletes its key); the overlay is closed. */
function overlayOnto(t, to, pb) {
  for (const key of Reflect.ownKeys(pb)) t.sc === 2 ? (to[key] = pb[key]) : copyOwn(to, pb, key);
  if (t.del !== null) {
    for (const key of t.del) delete to[key];
    t.kc -= t.del.size;
    t.del = null;
  }
  t.ovl = false;
  return to;
}
/** Adoption (a setter's returned replacement; reconcile and projection
 * landings in S3): the incoming object becomes the backing by reference at
 * the fold — no clones, no user-object mutation, ownership resets to
 * shared. It is the container's staging until then (signal parity: a
 * handler reads the committed frame until the flush; a hold keeps it
 * staged with the rest). The nodes are told now, diffed against the view
 * they were LAST told (#3296): the draft's pending backing when one
 * preceded the adoption this batch — its node writes were real, and the
 * adoption's re-stage cancels or confirms each — else the committed
 * backing. */
function adoptPB(target, incoming, notify = true) {
  if (!directCommit) queueFold(target);
  let base;
  if (target.pb !== null) {
    if (target.ovl) materializePB(target);
    base = target.pb;
  } else base = target.v;
  // #4146 for an adoption (#5005): before a user's adoption swaps a HELD
  // container's staging, its keys get their leaves, born from the two frames,
  // so a change stages on its key as a trap write's would (an array's keys
  // stage as its setter's do).
  if (userWrite && target.k?._config & CONFIG_HELD && !Array.isArray(base))
    for (const key in { ...base, ...incoming }) deferredReleases.add(getNode(target, key));
  if (directCommit) {
    target.pb = null;
    if (target.k !== null) target.k._value = incoming;
  } else {
    // Eager (INTERNALS §3): the backing swaps now — handlers read the
    // adopted object before the flush — while the container node, when
    // there is one (`stageOn`), keeps the committed frame (`_value`) for
    // readers a hold keeps on it, and stages the adoption for the fold's
    // path copy.
    target.pb = incoming;
    stageOn(target, incoming);
  }
  target.v = incoming;
  target.ch = incoming[$TARGET] !== undefined;
  target.ovl = false;
  target.del = null;
  target.sc = 0;
  target.a = false;
  target.wk = null; // adoption supersedes staged trap writes
  const owner = incoming[$OWNER];
  if (owner?._fam === target._fam) incoming[$OWNER] = target;
  else setLookup(incoming, target);
  // S2: the landing reaches each live guess carrier as the truth it is
  // judged against (a returned-rows derive never passes `notifyWrites`).
  if (optHooks !== null && target.o) optHooks.judge(target, incoming);
  if (notify && base !== incoming) notifyFold(target, base, incoming);
}
// ---------------------------------------------------------------------------
// the fold: owned raw's one mutation point, at the flush's commit
/** The fold queue: the targets with a batch open (`pb !== null` IS the
 * membership — a target is queued exactly while it carries a staging; the
 * drain skips one already folded), in two reusable arrays. Nothing here
 * allocates per batch once warm: a dbmon tick queues ~7000 containers, and
 * a map built and drained per tick cost ~540 KB of table allocation — most
 * of the tick's young-generation GC, and the whole of its gap to `next`
 * (plan sec. 41.5).
 *
 * Beside each target, its PRE-BATCH committed backing — `t.v` as the batch
 * opened on it (`foldOlds`, parallel to `foldList`). That one record is the
 * fold's base in every case: a draft's backing does not move until the fold
 * (so the record is `t.v` still), an adoption swaps it eagerly (the record
 * is the frame committed readers keep), a mid-batch privatization or a
 * draft over an adopted raw moves it again (the record is what the batch
 * started from, which is all the fold wants to know). The record lives
 * exactly as long as the batch: a weak map keyed by target kept every
 * container's LAST adopted-away backing alive until its next adoption —
 * for a keyed reconcile, the whole previous tree promoted out of the
 * nursery every tick (the saturated listened-paths shape ran 15–25% over
 * the fork, all of it this; plan sec. 43.2) — and for a store adopted
 * once, for its lifetime.
 *
 * The target carries no slot for its record,
 * so a lookup by target (`preBatch`: a container node born onto an open
 * staging, a committed-frame read of a node-less staging, a park) indexes
 * the list through a map built on first use per batch — the drain itself
 * walks the two lists in step and never builds it. */
let foldList = [];
let foldOlds = [];
let foldSpare = [];
let foldOldsSpare = [];
let foldIndex = null;
/** Before the staging is set and before an adoption swaps the backing. */
function queueFold(target) {
  if (target.pb !== null) return;
  schedule();
  if (foldIndex !== null) foldIndex.set(target, foldList.length);
  foldList.push(target);
  foldOlds.push(target.v);
}
/** The committed backing before this batch (valid while `pb !== null`). */
function preBatch(t) {
  if (foldIndex === null) {
    foldIndex = new Map();
    for (let i = 0; i < foldList.length; i++) foldIndex.set(foldList[i], i);
  }
  const at = foldIndex.get(t);
  return at === undefined ? t.v : foldOlds[at];
}
/** The flush committed its pending nodes: fold every pending backing whose
 * container node committed (its staging is gone — `_value` is the backing
 * now) into the committed home; one a later step holds (its node still
 * staged) waits for the flush that commits it. Then the deferred slot
 * releases. */
function drainFolds() {
  if (foldList.length !== 0) {
    const list = foldList;
    const olds = foldOlds;
    foldList = foldSpare;
    foldOlds = foldOldsSpare;
    foldSpare = list;
    foldOldsSpare = olds;
    foldIndex = null;
    for (let i = 0; i < list.length; i++) {
      const t = list[i];
      if (t.pb === null) continue; // folded already (a direct commit)
      const k = t.k;
      if (k !== null && k._pendingValue !== NOT_PENDING) {
        // held: commits with its hold
        foldList.push(t);
        foldOlds.push(olds[i]);
      } else foldTarget(t, olds[i]);
    }
    list.length = 0;
    olds.length = 0;
  }
  if (deferredReleases.size !== 0) {
    for (const node of deferredReleases) {
      if (node._subs === null && !slotKept(node)) {
        deferredReleases.delete(node);
        releaseSlot(node);
      }
    }
  }
}
/** Core asks: is a slot node's truth in flight? Its family's derive is —
 * lanes (A17, #2864: a guess mid-refetch stands in for the flight) and
 * marks (an ambient `affects()` over a declared reload lives to the
 * landing) read it. */
GlobalQueue._slotFlight = n => {
  const fam = n._host?._fam;
  return (fam?.node?._statusFlags & STATUS_PENDING) !== 0;
};
installStoreHooks(storePark, () => {
  drainFolds();
  optHooks?.sweep();
});
/** The store half of `affects()` (store/affects.ts) makes its marks' nodes
 * and walks records through these: it never imports the engine. */
GlobalQueue._storeNode = getNode;
GlobalQueue._storeWrappable = isWrappable;
/** A projection's creation run commits directly (a memo's first value is
 * its `_value`, not a staging): the draft still writes a clone of the seed,
 * nothing is staged on the nodes, and the setter's exit folds the touched
 * containers at once. Only the synchronous first run. */
let directCommit = false;
function runDirect(fn) {
  const was = directCommit;
  directCommit = true;
  try {
    fn();
  } finally {
    directCommit = was;
  }
}
/** Fold one target's pending backing into its committed home. */
function foldTarget(t, old) {
  {
    {
      const k = t.k;
      const pb = t.pb;
      if (pb !== null) {
        if (pb === t.v);
        else if (
          t.ovl &&
          (t.v !== old || (!overlayRebuilds(t, pb) && k?._x?._snapshotValue !== old))
        ) {
          // Overlay flatten (#3044): the backing keeps its identity, so the
          // `t.v === old` gate below skips path copying (the parent slot
          // already points here) and the adopted-notify (setter
          // notifications happened at write time). A backing privatized
          // mid-batch is a fresh clone the parent already points at.
          flattenOverlay(t, pb);
        } else if (t.v !== old) {
          // Privatized mid-batch (a child's path copy repointed us): the
          // owned backing takes the written keys.
          privatizeCommitted(t);
          const v = t.v;
          const wk = t.wk;
          if (wk !== null && wk !== WK_ALL) {
            for (let w = wk; w !== undefined; w = w.u)
              for (const key of w) {
                if (hasOwn$1.call(pb, key)) copyOwn(v, pb, key);
                else delete v[key];
              }
          } else {
            for (const key of Reflect.ownKeys(pb)) if (key !== $OWNER) copyOwn(v, pb, key);
            for (const key of Reflect.ownKeys(old)) {
              if (key !== $OWNER && !hasOwn$1.call(pb, key)) delete v[key];
            }
          }
        } else {
          // The clone (or adopted object) becomes the committed backing
          // (path copy below).
          t.v = t.ovl ? materializePB(t) : pb;
          t.ch = t.v[$TARGET] !== undefined;
        }
        t.pb = null;
        t.wk = null; // the written-keys window closes with the commit
      }
      if (k !== null) {
        // The node's committed value is the backing. A container nobody
        // subscribes to was a transient staging home (INTERNALS §5: an
        // observer-less write holds a transient record until the fold,
        // discarded with it) — gone with the batch.
        if (k._subs === null) t.k = null;
        else k._value = t.v;
      }
      if (t.v !== old && t.u) {
        const slot = parentSlotKey(t, old);
        if (t.u.v[slot] === old) {
          privatizeCommitted(t.u);
          t.u.v[slot] = t.v;
        }
      }
    }
  }
}
// ---------------------------------------------------------------------------
// write-time notification (setter exit)
/** A user setter wrote a key whose staging is a derivation another
 * transaction holds (A34 (3), #3612): the hold re-derives over the write. */
let heldDerivationHit = false;
/** The open setter is a USER'S (`storeSetter(…, guard = true)`), not the
 * derive's draft or landing. */
let userWrite = false;
/** A hold whose staging the outermost user setter read from its draft (#5046). */
let draftReadTx = null;
/** Is the open setter a user's? (reconcile.ts: a user's reconcile on an
 * optimistic family writes the draft.) */
function userWriting() {
  return userWrite;
}
function heldDerivation(t, node, key) {
  if (t._fam === null || !userWrite) return false;
  if (node !== undefined)
    return (
      (node._config & (CONFIG_HELD | CONFIG_MANUAL_WRITE)) === CONFIG_HELD &&
      txOf(node) !== flushTransaction
    );
  const k = t.k;
  return (
    k !== null &&
    (k._config & CONFIG_HELD) !== 0 &&
    txOf(k) !== flushTransaction &&
    heldKeyChanged(t, key)
  );
}
/** Setter exit: one node write per changed observed key (the leaves stage
 * like signals — holds and lanes engage here on later steps), presence and
 * deep witnesses, and the container when membership changed. */
function notifyWrites(t) {
  let pb = t.pb;
  if (pb === null) return;
  // Optimistic channel: a user's writes on an optimistic family become
  // guesses on the nodes (lanes); the draft is discarded, the committed
  // backing never touched.
  if (optHooks !== null && t.o) {
    if (userWrite) {
      // The draft is discarded first: the diff reads the target (its nodes
      // are born from the committed frame, not the draft).
      t.pb = null;
      t.wk = null;
      t.pb = optHooks.writes(t, pb);
      return;
    }
    // The derive's draft: new rows under guessed rows are paired with them
    // first (D-34, #4329); the frame then reaches each live guess carrier as
    // the truth it is judged against (S2).
    optHooks.pair(t, pb);
    optHooks.judge(t, pb);
  }
  const old = t.v;
  // Devtools mutation hook: full-key diff (dev-only cost) so unobserved
  // writes report too. Overlay backings materialize first so the diff walks
  // a real container.
  if (DEV$1.hooks.onStoreNodeUpdate) {
    if (t.ovl) materializePB(t);
    pb = t.pb;
    for (const key of Reflect.ownKeys(pb)) {
      if ((Array.isArray(pb) && key === "length") || key === $OWNER) continue;
      const ov = old[key];
      const nv = pb[key];
      if (!isEqual(ov, nv)) DEV$1.hooks.onStoreNodeUpdate(t.px, key, nv, ov);
    }
    for (const key of Reflect.ownKeys(old)) {
      if (key in pb || key === $OWNER) continue;
      DEV$1.hooks.onStoreNodeUpdate(t.px, key, undefined, old[key]);
    }
  }
  const nodes = t.n;
  // Written-keys bound: trap writes record their keys, so the notify visits
  // O(written) nodes instead of every subscription on the record (a selection
  // map with thousands of per-key subscribers pays two visits per select,
  // not a full scan). Falls back to the full node scan when the bound can't
  // hold: no trap granularity (wk null), an array length write (WK_ALL —
  // implicit index deletes), accessors on the record (t.a — a getter node's
  // value can change when ANY key is written), or a non-plain prototype
  // (class instances: prototype getters derive from arbitrary fields).
  // Overlay pbs chain to the COMMITTED object (#3044) — plainness is the
  // committed container's prototype, not the overlay's. THIS setter's keys
  // only (D-8, #3743): a key an earlier setter of the batch wrote and this
  // one did not is no second proposal — its node was told then, and telling
  // a held node its unchanged value again would join this tick to the hold.
  const wk0 = t.wk;
  const writtenKeys = wk0 === WK_ALL || t.a === true || !plainProto(t.ovl ? t.v : pb) ? null : wk0;
  if (attrHooks !== null) attrHooks.storeWrite(t, old, pb, writtenKeys);
  if (t._fam !== null && !heldDerivationHit && writtenKeys !== null)
    for (const key of writtenKeys)
      if (heldDerivation(t, nodes?.[key], key)) {
        heldDerivationHit = true;
        break;
      }
  if (nodes !== null) {
    const keys = writtenKeys ?? Reflect.ownKeys(nodes);
    for (const key of keys) {
      const node = nodes[key];
      if (node === undefined) continue;
      let own = false;
      if (
        node.acc === true ||
        ((own = hasOwn$1.call(pb, key)) && lookupGetter.call(pb, key) !== undefined)
      ) {
        // Accessor keys: the node is linked for shape-change notification,
        // its value is never served (the getter runs with the proxy
        // receiver on read) — FORCE wakes the readers. A data descriptor
        // falls through so the cleared-acc hot read serves it (#3949);
        // an own getter's descriptor has no `value`, so the compare below
        // would otherwise keep the getter's result (or FORCE) for `undefined`.
        node.acc = isAccessorKey(pb, key);
        const od = Object.getOwnPropertyDescriptor(old, key);
        const nd = Object.getOwnPropertyDescriptor(pb, key);
        if (!nd || nd.get || nd.set) {
          if (od?.get !== nd?.get || od?.set !== nd?.set || od?.value !== nd?.value)
            setSignal(node, () => FORCE);
          continue;
        }
        if (od?.get || od?.set || !isEqual(od?.value, nd.value)) setSignal(node, () => nd.value);
        continue;
      }
      // A removed `__proto__` is told `undefined`, never the prototype (core
      // R30, #4672); an own key pays nothing. (An overlay's written keys are
      // own on it or in `del`.)
      const nv = own || !(t.del?.has(key) || key === "__proto__") ? pb[key] : undefined;
      // A key a live guess covers: its readers read the carrier, and the
      // carrier's judgement of this landing tells them — a correction through
      // the carrier, a confirmation silently (A17). The leaf records the
      // value and says nothing (S2).
      if (optHooks !== null && t.o && optHooks.covers(t, key, false)) {
        coveredSet(node, nv);
        continue;
      }
      setSignal(node, () => nv);
      if (t._fam !== null) {
        // The derive's write is a derivation; a user setter's is a proposal
        // (A34 (3)'s discriminator).
        if (!userWrite) {
          node._config &= ~CONFIG_MANUAL_WRITE;
          holdWithDerive(t, node);
        } else node._config |= CONFIG_MANUAL_WRITE;
      }
    }
  }
  const has = t.h;
  if (has !== null) {
    const keys = writtenKeys ?? Reflect.ownKeys(has);
    for (const key of keys) {
      const node = has[key];
      if (node !== undefined) {
        if (optHooks !== null && t.o && optHooks.covers(t, key, true)) {
          coveredSet(node, key in pb && !t.del?.has(key));
          continue;
        }
        setSignal(node, key in pb && !t.del?.has(key));
        if (t._fam !== null && !userWrite) holdWithDerive(t, node);
      }
    }
  }
  if (deepUsed && t.dk !== null) {
    if (t.del?.size) bumpDeep(t);
    else
      for (const key of writtenKeys ?? Reflect.ownKeys(pb)) {
        if (key === $OWNER) continue;
        const nv = pb[key];
        const ov = old[key];
        if (nv !== null && typeof nv === "object" ? !targetsEqual(ov, nv) : !isEqual(ov, nv)) {
          bumpDeep(t);
          break;
        }
      }
  }
  // The container: structural readers hear membership/arrangement changes
  // only (R9) — the compare is the store's, against the committed frame. A
  // container carrying an arrangement guess is told whether or not anyone
  // subscribes: the write is a landing on the guess (`notifyContainer`).
  const k = t.k;
  if (k !== null && (k._subs !== null || k._config & CONFIG_OVERRIDE)) {
    let changed;
    if (t.ovl) {
      changed = !!t.del?.size;
      if (!changed) {
        for (const key of Reflect.ownKeys(pb)) {
          if (!hasOwn$1.call(old, key)) {
            changed = true;
            break;
          }
        }
      }
    } else {
      changed =
        Array.isArray(pb) && Array.isArray(old)
          ? arrayStructureChanged(old, pb)
          : membershipChanged(old, pb);
    }
    if (changed) notifyContainer(k, pb);
  }
}
/** A covered leaf records the truth and says nothing (S2). A staging it
 * was born with or held from an older landing (`bornStaged`) is that older
 * truth: the hold's commit lands this one, never the older one once the
 * guess ends (#4904). */
function coveredSet(node, v) {
  node._value = v;
  if (node._pendingValue !== NOT_PENDING) node._pendingValue = v;
}
/** Accessor-node wake sentinel: readers re-run and re-invoke the getter. */
const FORCE = Symbol();
/** Two raws that wrap to the same target are the same logical value. */
function targetsEqual(ov, nv) {
  if (ov === null || typeof ov !== "object" || nv === null || typeof nv !== "object") return false;
  const ot = lookupTarget(ov, null);
  return ot !== undefined && ot === lookupTarget(nv, null);
}
/** Positional identity IS content for arrays (R15). */
function arrayStructureChanged(old, neu) {
  if (old.length !== neu.length) return true;
  for (let i = 0; i < neu.length; i++) {
    const ov = old[i];
    const nv = neu[i];
    if (!isEqual(ov, nv) && !targetsEqual(ov, nv)) return true;
  }
  return false;
}
function membershipChanged(old, neu) {
  const nk = Reflect.ownKeys(neu);
  if (Reflect.ownKeys(old).length - +isOwned(old) !== nk.length - +isOwned(neu)) return true;
  for (const key of nk) if (key !== $OWNER && !(key in old)) return true;
  return false;
}
/** An adoption's leaf write. A derive's pass that is a lane's work (a
 * display-ahead pass, F-6) writes the lane's derivation (D-25), as its draft
 * writes do (`notifyWrites` → `holdWithDerive`): the lane's value, the
 * committed value — and a truth a transaction holds beneath — untouched
 * (#4631: staged into the hold instead, the leaf showed the screen beside a
 * container showing the lane). */
function leafSet(node, v) {
  // A key a live guess covers: its readers read the carrier, and the
  // carrier's judgement of this landing tells them — a correction through
  // the carrier, a confirmation silently (A17). The leaf records the value
  // and says nothing (S2).
  const t = node._host;
  if (optHooks !== null && t.o && optHooks.covers(t, node._key, t.h?.[node._key] === node)) {
    coveredSet(node, typeof v === "function" ? v() : v);
    return;
  }
  setSignal(node, v);
  if (passLane !== null && !userWrite && node._host._fam !== null) holdWithDerive(node._host, node);
}
/** One leaf's notification for an adoption diff (descriptor-aware). */
function notifyKeyDiff(node, key, old, neu, probe = true) {
  if (
    node.acc === true ||
    (probe && hasOwn$1.call(neu, key) && lookupGetter.call(neu, key) !== undefined)
  ) {
    node.acc = isAccessorKey(neu, key);
    const od = Object.getOwnPropertyDescriptor(old, key);
    const nd = Object.getOwnPropertyDescriptor(neu, key);
    // Data descriptors fall through, as in notifyWrites (#3949).
    if (!nd || nd.get || nd.set) {
      if (od?.get !== nd?.get || od?.set !== nd?.set || od?.value !== nd?.value)
        leafSet(node, () => FORCE);
      return;
    }
    const ov = od?.value;
    const nv = nd?.value;
    if (od?.get || od?.set || (!isEqual(ov, nv) && !targetsEqual(ov, nv)))
      leafSet(node, typeof nv === "function" ? () => nv : nv);
  } else {
    const ov = old[key];
    const nv = neu[key];
    // A live chained `old` (a store proxy, §7b) already reads the inner
    // store's next frame, not what the node was last told: written, the
    // node's own equality judging it (#4361).
    if (old[$TARGET] || (!isEqual(ov, nv) && !targetsEqual(ov, nv)))
      leafSet(node, typeof nv === "function" ? () => nv : nv);
  }
}
function hasAccessorFlag(node) {
  return node.acc === true;
}
/** One leaf's notification when the caller already has both values. */
function notifyKeyValue(node, key, ov, nv, old, neu) {
  if (node.acc === true) {
    notifyKeyDiff(node, key, old, neu, false);
    return;
  }
  if (!isEqual(ov, nv) && !targetsEqual(ov, nv))
    leafSet(node, typeof nv === "function" ? () => nv : nv);
}
/** The structural half of an adoption's notifications (reconcile does the
 * leaves itself): presence by `in`, the container by membership. Presence
 * is DIFFED `old` → `neu` like the leaves (#3743): a presence node is
 * written only where `in` changed between the view it was last told
 * (#3296) and the adoption — `setSignal` joins a held node's transaction
 * before its equality gate (A34 (1)), so repeating an unchanged absence to
 * a node an action holds would make the whole tick the action's (an
 * unrelated `a.value` stayed stale until the action settled). A real
 * change still writes, and on a held node still proposes. A live chained
 * `old` (a store proxy, §7b) reflects the inner store, not what the node
 * was last told — written unconditionally, as before; its container too
 * (#4361).
 *
 * The container is told of an arrangement change when it has structural
 * subscribers OR carries an arrangement guess (S4): for a guessed
 * container the write is the landing that judges the guess (Q-D, plan
 * sec. 39 — confirm, supersede, or hold beneath), not a subscriber
 * notification, and a landing whose truth changed no guessed key's value
 * or presence (a newer question's rows beneath an optimistic push) still
 * answers the arrangement — before, only the presence write reached the
 * lane, by accident of being unconditional. */
function notifyFoldTail(t, old, neu) {
  const has = t.h;
  const live = old[$TARGET];
  if (has !== null) {
    for (const key of Reflect.ownKeys(has))
      if (live || key in old !== key in neu) leafSet(has[key], key in neu);
  }
  const k = t.k;
  if (k !== null && (k._subs !== null || k._config & CONFIG_OVERRIDE)) {
    const changed =
      live ||
      (Array.isArray(neu) && Array.isArray(old)
        ? arrayStructureChanged(old, neu)
        : membershipChanged(old, neu));
    if (changed) notifyContainer(k, neu);
  }
}
/** An adoption's notifications: every node of the target, diffed `old` →
 * `neu` (value nodes by key, presence by `in`, the container by
 * membership, the deep witness by identity). */
function notifyFold(t, old, neu) {
  if (deepUsed && t.dk !== null && old !== neu) bumpDeep(t);
  const nodes = t.n;
  if (nodes !== null) {
    for (const key of Reflect.ownKeys(nodes)) notifyKeyDiff(nodes[key], key, old, neu);
  }
  notifyFoldTail(t, old, neu);
}
// ---------------------------------------------------------------------------
// visibility
let writing = 0;
/** Roots of the stores with an open setter: reads of THEIR drafts are the
 * writer's (no tracking, pending backing served); every other store reads
 * normally — a derive's external reads are its dependencies (#3037). */
let writeScopes = null;
const pendingNotify = new Set();
function scopeKey(target) {
  if (target._fam !== null) return target._fam;
  let t = target;
  while (t.u !== null) t = t.u;
  return t;
}
function inDraft(target) {
  return writeScopes !== null && writeScopes.has(scopeKey(target));
}
/** Shallow store: values served verbatim (a nested store proxy still wraps
 * into the family so writes stay isolated). */
function serveShallow(target, key, v) {
  if (v !== null && typeof v === "object" && v[$TARGET] !== undefined)
    return draftServe(target, wrap(v, target, key));
  return v;
}
/** A child proxy served to a draft joins the draft's scope. */
function draftServe(target, proxy) {
  if (writeScopes !== null && inDraft(target)) {
    const ct = proxy?.[$TARGET];
    if (ct?.v !== undefined) writeScopes.add(scopeKey(ct));
  }
  return proxy;
}
/** The committed backing: the container node's committed value while a
 * batch is staged on it (an adoption swaps `t.v` eagerly — INTERNALS §3,
 * "adoption is eager by contract" — and the node keeps the frame committed
 * readers under a hold still see), `t.v` otherwise. */
function committed(t) {
  const k = t.k;
  if (k !== null) return k._pendingValue !== NOT_PENDING ? k._value : t.v;
  // A node-less staging (`stageOn`): the fold queue remembers the frame.
  return t.pb !== null ? preBatch(t) : t.v;
}
/** The pass that sees this flush's staging (signal parity — `read()`'s
 * staged arm): the computation running (`context`, never the owner — a
 * handler, an effect callback or `onSettled` has an owner but no pass),
 * unless it is a frame reader (children-forbidden, A32). Null = the
 * committed frame. */
function stagingReader() {
  let c = context;
  if (c === null) return null;
  if (c._root) c = c._parentComputed ?? null;
  return c === null || c._config & CONFIG_CHILDREN_FORBIDDEN ? null : c;
}
/** The backing a read serves (RUL-1, signal parity): the draft and the write
 * override read the pending backing; a tracked reader reads it too — only
 * for presence and descriptors, its leaf (born from the two frames) decides
 * the value through core; a context-free or frame reader the committed
 * backing; a pass reading UNTRACKED asks the container node — `read()`
 * without a link: the node's value is the backing, chosen by the hold rules
 * (a render effect outside the parking flush sees committed and re-derives
 * at the landing; a memo joins the future; a staging is marked read). */
/** The backing a read of `target` is served from — the truth (committed,
 * or the staging a pass may see). An optimistic family's lane values are
 * NOT composed here: a guessed key always has its node, and the node is
 * what serves its value (`nodeValue`); presence and enumeration compose
 * per key / per walk (`optimisticHas`, `enumerationSource`). */
function readSource(target, key, shape = false) {
  // An optimistic setter's read before its first write sees what the writer
  // sees — the lanes' values and the tick's own guesses over the truth
  // (#3665), never a staging awaiting its fold: the draft is born on the
  // read (and notified at the setter's exit like any).
  if (optHooks !== null && userWrite && target.o && !pendingNotify.has(target) && inDraft(target)) {
    const draft = ensurePB(target);
    pendingNotify.add(target);
    return draft;
  }
  const pb = target.pb;
  if (pb === null) return target.v;
  if (inDraft(target) || writeOverride) {
    // The derive reads its own store through its draft: a key (or the
    // container) a transaction holds — a user's write inside an action,
    // held with it — is the future, and the pass that reads it derives
    // from the future (L2: a read of something already held joins it): the
    // derive's whole result reveals with the action (core R31 / #3733:
    // "under the transaction and revealed with it"), not key by key. A
    // user's setter reading its draft is no pass: it notes the hold, and
    // its writes join it at the setter's exit (#5046, as D-33's guess
    // written after reading another hold's guess does).
    if (userWrite !== writeOverride) {
      const leaf = key !== undefined ? target.n?.[key] : undefined;
      const k = leaf ?? target.k;
      if (
        k != null &&
        (k._config & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD &&
        (leaf !== undefined || key === undefined || heldKeyChanged(target, key))
      )
        userWrite ? (draftReadTx = txOf(k)) : joinPassTx(txOf(k));
    }
    return pb;
  }
  const k = target.k;
  // A key whose leaf carries a lane's value (a guess, S4): the leaf serves
  // it — the container's frame is not consulted (a reader is not made a
  // stale reader of the container's hold for a key it does not see through
  // it). Unless the container carries the same lane's value: then the
  // container's frame is the one the leaf is in, and presence and keys read
  // it, as the leaf's value does (#4821: a projection a lane pass derived).
  // (A leaf whose lane is gone matches no container: `|| 0`.)
  if (key !== undefined) {
    const leaf = target.n?.[key];
    if (leaf?._config & CONFIG_OVERRIDE && k?._x?._transaction !== (leaf._x._transaction || 0))
      return committed(target);
  }
  const held = k !== null && (k._config & CONFIG_HELD) !== 0;
  // A held container: the hold's unit is the key (#3706). A key the HELD
  // staging left unchanged reads the ambient view — a mainline layer's
  // write above the hold publishes mainline (#3688, #3612); a key it
  // changed reads the frame core serves this reader.
  if (held && key !== undefined && !heldKeyChanged(target, key, shape)) return pb;
  const c = stagingReader();
  if (c === null) {
    if (getObserver() !== null) return pb; // a frame reader's leaf decides the value
    // A projection's writes are its truth as soon as made (the derive is
    // the authority, nothing it writes is a proposal): a context-free reader
    // sees them before the flush — unless a hold keeps them staged.
    if (familyAhead(target)) return pb;
    // A verdict window (`isPending`/`latest`) with no reader (a top-level
    // probe) judges the container like any node — for a key the batch
    // changed; an unchanged key is committed and final (A22: pending is per
    // key). A held container's committed frame is the node's.
    if (verdict === null || (!held && key !== undefined && !keyChanged(target, key, shape)))
      return held ? k._value : target.v;
    return k === null ? committed(target) : read(k);
  }
  // A pass reading a key the batch left unchanged: the same value in both
  // frames — committed, holding no one. Otherwise the container's frame by
  // core's rules — `read()` of the node WITHOUT a link (a value reader never
  // subscribes to the container): a render effect outside a hold's flush
  // sees committed and re-derives at the landing, a memo joins the future,
  // a staging read is marked. The key's own leaf, when it has one, decides
  // the value the same way.
  if (!held) {
    // A plain pass this flush already served a staging as the screen
    // (REACTIVE_STAGED_READ — what `read()` would set again here) reads an
    // unheld staging directly: an unchanged key is the same value in both
    // frames, a changed one is what the mark already stands for. The
    // per-key question is asked once per pass, not once per key (a
    // `mapArray` over 1k shifted rows asked it 1k times).
    if (
      c._flags & REACTIVE_STAGED_READ &&
      !(c._config & (CONFIG_VERDICT | CONFIG_CHILDREN_FORBIDDEN))
    )
      return pb;
    if (key !== undefined && !keyChanged(target, key, shape)) return committed(target);
    // A node-less staging (`stageOn`): what `read()` of its node would do
    // for this pass — a verdict reader sees the screen, any other derives
    // from the staging and is marked as having read it.
    if (k === null) {
      if (c._config & CONFIG_VERDICT && stagedScreen(c)) return committed(target);
      stagedRead(c);
      return pb;
    }
  }
  return untrack(() => read(k));
}
/** Did the HELD staging (the container node's, not a mainline layer above
 * it) change `key`? */
function heldKeyChanged(target, key, shape = true) {
  return (
    changedBetween(target, target.k._value, target.k._pendingValue, key, shape) &&
    !adoptChild(target, key)
  );
}
/** The values of a held frame, built once per frame: neither the committed
 * frame nor the held staging changes while the hold stands (a draft writes a
 * clone), so the set stays true for every first read of the hold. */
const frameRaws = new WeakMap();
function rawsOf(frame) {
  let raws = frameRaws.get(frame);
  if (raws === undefined) frameRaws.set(frame, (raws = new Set(Object.values(frame))));
  return raws;
}
/** The same row at a slot, as the adoption pairs rows (reconcile.ts): two
 * objects whose keys agree — positionally for an unkeyed family. */
function sameRow(pv, nv, keyFn) {
  return (
    pv !== null &&
    typeof pv === "object" &&
    nv !== null &&
    typeof nv === "object" &&
    (keyFn === null || sameKey(keyFn(pv), keyFn(nv)))
  );
}
/** A key of a held adoption whose old and new values are the same row that
 * nothing had proxied when the adoption ran, so `descend` never paired
 * them: paired now, as it would have been (#3712) — one target, the old
 * raw its committed frame, the new raw its staging, held by the same
 * transaction. Both raws resolve to it, so the slot reads as unchanged and
 * the row's own leaves decide: `id` the same in both frames holds no one,
 * `version` holds. A record's keys pair by name; an array's by the family's
 * key — without a family the pairing is not known here and the hold stands. */
function adoptChild(target, key) {
  const k = target.k;
  const old = k._value;
  const ov = old[key];
  const nv = k._pendingValue[key];
  const keyFn = Array.isArray(old) ? target._fam?.key : null;
  if (
    target.s ||
    keyFn === undefined ||
    !sameRow(ov, nv, keyFn) ||
    !isWrappable(ov) ||
    !isWrappable(nv) ||
    Array.isArray(ov) !== Array.isArray(nv) ||
    lookupTarget(ov, target._fam) !== undefined ||
    lookupTarget(nv, target._fam) !== undefined ||
    // A raw in both frames (the same objects swapped or reordered) is its
    // own row: pairing it with another would alias the two after the commit.
    rawsOf(old).has(nv) ||
    rawsOf(k._pendingValue).has(ov)
  )
    return false;
  // A raw-marked value wraps to itself, a proxy to its own target: neither
  // is a fresh target around `ov`.
  const ct = wrap(ov, target, key)[$TARGET];
  if (ct?.v !== ov) return false;
  adoptPB(ct, nv, false);
  holdNode(getContainerNode(ct), txOf(k));
  return true;
}
/** A family target's unheld staging, read with no pass (a handler). */
function familyAhead(target) {
  return target._fam !== null && context === null && !(target.k?._config & CONFIG_HELD);
}
/** Did this batch change `key` — value (slot equality), presence, and (for
 * a `shape` reader — a descriptor read, a node's hold) enumerability or
 * accessor-ness? A swapped or non-plain prototype, or a chained backing,
 * changes every key (#3706: those hold the whole container).
 *
 * A VALUE read asks without `shape`: a key whose value is the same in both
 * frames serves the same value from either, whatever its descriptor did —
 * and the descriptor probes (`__lookupGetter__`, `propertyIsEnumerable`,
 * twice each) are the cost of the question on a wide container read under
 * a staging (a keyed reconcile's 1k rows, read by `mapArray` in the flush
 * that staged them: ~3× the tick). A container with accessors seen (`a`)
 * keeps the full test — a getter's answer is not a slot's. */
function keyChanged(target, key, shape = true) {
  return changedBetween(target, committed(target), target.pb, key, shape);
}
function changedBetween(target, v, pb, key, shape) {
  if (target.ch || pb[$TARGET] !== undefined || protoOf(pb) !== protoOf(v) || !plainProto(v))
    return true;
  if (target.del?.has(key)) return key in v;
  const inNew = hasOwn$1.call(pb, key) || (target.ovl && hasOwn$1.call(target.v, key));
  if (inNew !== hasOwn$1.call(v, key)) return true;
  if (!inNew) return false;
  if (
    (shape || target.a) &&
    (isOwnAccessor(v, key) ||
      isOwnAccessor(pb, key) ||
      propertyIsEnumerable$1.call(v, key) !== propertyIsEnumerable$1.call(pb, key))
  )
    return true;
  const nv = pb[key];
  const ov = v[key];
  return !isEqual(ov, nv) && !sameLogicalSlot(target, ov, nv);
}
/** Value of a key with a leaf: core's — `read()` links a tracked reader and,
 * tracked or not, applies the hold and A28 rules a signal's read does. A
 * projection's unheld staging is ahead for a handler (`familyAhead`), and is
 * the only value while its derive has never committed: the committed side
 * is the seed, never observable (A25) — a reader core serves the screen (a
 * verdict reader, #4355) is served the staging, as for a memo born staged
 * (core `serve`). */
function nodeValue(node, backing, target) {
  let v;
  if (
    node._pendingValue !== NOT_PENDING &&
    !(node._config & CONFIG_HELD) &&
    target._fam !== null &&
    (context === null || target._fam.node?._statusFlags & STATUS_UNINITIALIZED)
  ) {
    // A pass over a derive that has never committed: the staging is its
    // first answer, not a change — linked and served with no verdict (a
    // first landing is loading, not pending: A19 exc. 1, F-9, as for a memo
    // born staged; core's verdict would call the leaf's staging pending).
    devRead(node); // read()'s dev diagnostics, which this read skips
    if (context !== null && tracking) link(node, context);
    v = node._pendingValue;
  } else v = read(node);
  return v === FORCE ? backing : v;
}
/** Serve a present data key: link the leaf for a tracked reader (creating it
 * on first read), wrap a wrappable child through the per-node wrap cache. */
function serveDataKey(target, key, backingValue, src, node, accKnown = -1) {
  // Read-through (§7b): the inner store's node is the truth; the outer's is
  // a subscription point only (an optimistic target's guesses are served by
  // its carrier before this, #4762).
  const chained = target.ch && src === target.v;
  let v = backingValue;
  // A write-override read (a derive's draft) serves the backing: the derive
  // composes on the truth, never on a lane's or a hold's frame.
  if (!inDraft(target) && !writeOverride) {
    if (node === undefined && getObserver() !== null) node = getNode(target, key, accKnown);
    if (node !== undefined) {
      const nv = nodeValue(node, backingValue, target);
      if (chained) {
        // The link's committed value follows the inner store's (what a lane
        // over it covers — its reveal and revert compare against it, S4).
        node._value = backingValue;
      } else v = nv;
    }
  }
  // The tail is `serveKeyValue`, inlined: this is every store read's path,
  // and a call here showed on CodSpeed as −6 to −9% on the store reads
  // (#4729, third round); the optimistic hook's carrier read takes the
  // function.
  if (target.ch && !chained && v !== null && typeof v === "object" && v[$TARGET]?.px !== v)
    v = resolveChainedRaw(target, key, v);
  if (target.s) return serveShallow(target, key, v);
  if (node !== undefined) {
    if (node.pxv === v && v !== undefined) return draftServe(target, node.px);
    if (!isWrappable(v)) return v;
    const p = wrap(v, target, key);
    node.px = p;
    node.pxv = v;
    return draftServe(target, p);
  }
  if (!isWrappable(v)) return v;
  return draftServe(target, wrap(v, target, key));
}
/** The value of `key` as the reader gets it (the tail of `serveDataKey`, for
 * the optimistic hook's carrier read): shallow-served, a chained raw
 * resolved to the inner store's (#3767, #3859), a wrappable wrapped (cached
 * on the key's node by value identity). */
function serveKeyValue(target, key, v, chained, node) {
  // A derive's draft wrapper answers `$TARGET` without being a proxy: the
  // inner store's alias (#3767), resolved like a raw (#3859).
  if (target.ch && true && v !== null && typeof v === "object" && v[$TARGET]?.px !== v)
    v = resolveChainedRaw(target, key, v);
  if (target.s) return serveShallow(target, key, v);
  if (node !== undefined) {
    if (node.pxv === v && v !== undefined) return draftServe(target, node.px);
    if (!isWrappable(v)) return v;
    const p = wrap(v, target, key);
    node.px = p;
    node.pxv = v;
    return draftServe(target, p);
  }
  if (!isWrappable(v)) return v;
  return draftServe(target, wrap(v, target, key));
}
/** Is `c` among `n`'s subscribers (any pass's link — a kept tail included)?
 * `n`'s subs, not `c`'s deps: the derive's are its few flight observers. */
function linkedTo(c, n) {
  for (let s = n._subs; s !== null; s = s._nextSub) if (s._sub === c) return true;
  return false;
}
/** Does `n` read `c`, through any chain of computations? Only a chain of
 * nodes above `c` can reach it, so the walk stops at `c`'s height (a signal
 * has no height and ends it too). */
function feeds(n, c) {
  for (let l = n._deps; l; l = l._nextDep) {
    const d = l._dep;
    if (d === c || (d._height > c._height && feeds(d, c))) return true;
  }
  return false;
}
/** The projection's obligation (plan sec. 31.7 Q-B): before any value is served
 * through a family target, the derive — the firewall — is brought up to
 * date WITHOUT the reader subscribing to it. Core's `read()` of the
 * firewall with no link does all of it: the pull, the status gate
 * (uninitialized / pending → NotReady, errored → the error; a verdict
 * window judges it; a pending source registers the reader for the landing)
 * and the reader's height. The derive's own draft ops are exempt (the write
 * override): the derive is the author. */
function pullFamily(target) {
  const fw = target._fam.node;
  if (fw === null || writeOverride || inDraft(target)) return;
  // A reader the derive itself reads through (its row handed back into its
  // own source, #4970 / #4972) is the derive's input, not its reader: it
  // reads the leaves as the derive last wrote them, like any signal. Pulling
  // or linking the derive from it would be a cycle: each pass lifted it
  // above the derive that sits above it, without bound.
  const c = context;
  if (c?._subs && feeds(fw, c)) return;
  // A settled derive: the pull alone, no link (the barrier). A derive with a
  // flight up (or errored): the reader observes the flight exactly as a
  // reader of a memo would — linked, so it re-runs at the landing (the
  // next settled pass pulls without linking and the stale link trims), a
  // verdict reader registered, a tracked pass suspended.
  // A staged error (D-41) throws at the pull, before any leaf would link the reader.
  if (fw._statusFlags & (STATUS_PENDING | STATUS_ERROR) || fw._config & CONFIG_STAGED_ERROR) {
    // An optimistic family carrying guesses that stand in for the flight
    // (A17): the guesses are the display — a reader is served the leaves
    // (committed plus the guesses) and does not suspend on the derive; a
    // verdict window still asks it (`isPending` is true while the flight is
    // up). The family's, not the target's: the edit stands in for the one
    // flight the family has — not over a new question they do not answer
    // (optimistic.ts `stands`, #4336). A reader observing the flight keeps
    // observing it (`stands`, #4536). Judged first: the hook sees the frame's
    // readers too.
    if (verdict === null && fw._statusFlags & STATUS_PENDING && optHooks?.stands(target)) return;
    // A render effect outside the flight's own flush is the frame, not a
    // derivation: it keeps what it shows and learns of the landing from the
    // leaves the landing changes (unchanged leaves say nothing — it is not a
    // stale reader of the derive). Inside the flush it suspends like any.
    // One that already observes the flight (linked by the pass that saw it
    // go up — the frame's hold, `blocked`) keeps observing it: a re-run for
    // another reason (a guess it read, the seam) that dropped the link would
    // release the hold with the flight still up. An errored derive has no
    // landing to learn of: the error is what the frame shows.
    if (
      c !== null &&
      c._type === EFFECT_RENDER &&
      fw._statusFlags & STATUS_PENDING &&
      fw._config & CONFIG_HELD &&
      flushTransaction !== txOf(fw) &&
      !linkedTo(c, fw)
    )
      return;
    // A pass the pull throws a staged error (D-41) is linked to the derive,
    // untracked too — `handleAsync` asking a returned proxy whether it is a
    // flight: the pass carries the error, and the re-ask or answer that
    // supersedes it must reach it (#4527; a tracked read links anyway, #4519).
    // Only a computation has deps to link: a bare owner (`mapArray`'s row
    // update under `runWithOwner`, #4805) reads as untracked.
    if (c?._deps !== undefined && fw._config & CONFIG_STAGED_ERROR) link(fw, c);
    // D-30: a derive that is a verdict reader only through what it reads
    // (below) and waits on a flight it read as a lane's work: its reader
    // reads that flight as the memo's reader would. A branch a display-ahead
    // read mounted is born held on it (S-46) — read through the derive
    // alone, it waited with the lane and attached at the lane's reveal,
    // ahead of the landing its memo form attaches at.
    // (The condition is spelled out, as below: behind a helper the branch
    // does not fold away without the verdict engine.)
    if (
      observeFlightHook &&
      fw._config & CONFIG_VERDICT &&
      !(fw._flags & (EFFECT_RENDER << 21)) &&
      tracking
    )
      for (let l = fw._deps; l; l = l._nextDep)
        if (l._dep._statusFlags & STATUS_PENDING) read(l._dep);
    read(fw);
  } else {
    // #4782: the pull a tracked read makes — the derive brought up to date
    // now and the reader lifted above it — not only a woken one: a reader
    // of the leaves carries no height from the derive (a leaf has no `_fn`),
    // so the heap can run it first, against the derive's last answer.
    let r = context;
    if (r?._root) r = r._parentComputed;
    if (r) pullComputed(fw, r);
    untrack(() => read(fw));
    // #4491: a derive that is a verdict reader only through what it reads (a
    // verdict memo's value, D-25 / F-6; no window of its own) is served the
    // committed value of a flight it reads (`observeFlight`, A15's stale
    // reader) and settles over it: its leaves are the screen, not the
    // flight's answer. A derived store reads like its memo (D-30): a tracked
    // reader of the leaves reads that flight as the memo's reader would, so
    // one that mounts mid-flight consumes unanswered async and is held (F-2)
    // instead of shown the old question's answer. The derive is that hold's
    // work until it answers (D-25): the read marks its next pass, which reads
    // plainly (`fam.w`, projection.ts), so the landing it computes is the one
    // the held reader shows. The mark is all it writes — the derive's posture
    // stays its passes' own (core `recompute`) — and a read that finds no
    // flight up clears it: a landing equal to the screen re-runs nothing, and
    // the mark does not outlive the flight. Only the verdict engine makes a
    // verdict reader (`observeFlightHook`: without it the branch folds away).
    // (The window flag is REACTIVE_WINDOW, spelled from EFFECT_RENDER: an
    // import of it keeps a dead copy in bundles without the store.)
    if (
      observeFlightHook &&
      fw._config & CONFIG_VERDICT &&
      !(fw._flags & (EFFECT_RENDER << 21)) &&
      tracking
    ) {
      const fam = target._fam;
      fam.w = 0;
      for (let l = fw._deps; l; l = l._nextDep)
        if (l._dep._statusFlags & STATUS_PENDING) ((fam.w = 1), read(l._dep));
    }
  }
}
/** A projection's writes are its derive's: a staging made while the derive
 * is held (a flight up — its continuation writes after an `await`, a
 * callback's late write) belongs to the derive's transaction and reveals
 * with it, never drained early — and answers the derive's question: a quiet
 * re-ask's landing is quiet through its reveal (A19 exc. 2; the leaf's own
 * classification, as core reads it — `heldNotFinal`). Callers pass only the
 * derive's own writes (its draft or landing), never a user setter's
 * (`userWrite`): a user's is a proposal (A34 (3)'s discriminator). */
function holdWithDerive(
  target,
  node,
  // The lane the staging is work of: the pass's, or — a leaf born from a lane
  // container's staging (`bornStaged`, #4451) — the container's.
  lane = passLane
) {
  const fw = target._fam?.node;
  // A pass that is a lane's work (a display-ahead read, F-6): its write is
  // the lane's derivation, as a memo's value would be (D-25) — the lane's
  // value, the committed value untouched beneath; the lane's end commits it.
  if (
    laneStageHook &&
    lane !== null &&
    node._pendingValue !== NOT_PENDING &&
    !(node._config & CONFIG_OVERRIDE)
  ) {
    list(node, lane);
    node._config |= CONFIG_OVERRIDE;
    return;
  }
  if (
    fw !== undefined &&
    fw !== null &&
    fw._config & CONFIG_HELD &&
    node._pendingValue !== NOT_PENDING &&
    // A node carrying a guess is the lane's: its staging is the truth beneath
    // the guess (`setSignal` → `laneWrite`), never re-homed by the store.
    !(node._config & (CONFIG_HELD | CONFIG_OVERRIDE))
  ) {
    holdNode(node, txOf(fw));
    node._x._reask = fw._x._reask;
  }
}
/** A chained backing's child (§7b): a raw the inner family owns, or holds
 * at this key, is served as the inner store's wrapper — never a fresh
 * raw-keyed one — so the chained targets are the same objects across
 * settled and pending views. */
function resolveChainedRaw(target, key, v) {
  const innerT = target.v[$TARGET];
  if (innerT.ch) {
    const iv = resolveChainedRaw(innerT, key, v);
    return iv === v ? v : wrap(iv, innerT, key);
  }
  const owned = lookupTarget(v, innerT._fam);
  if (owned !== undefined) return owned.px;
  if ((innerT.v[key] === v || innerT.pb?.[key] === v) && isWrappable(v))
    return wrap(v, innerT, key);
  return v;
}
/** The backing a structural read serves: a tracked reader subscribes to the
 * container and reads the frame core serves it — committed or staging by
 * the hold rules (`read()` on the node: the node's value IS the backing);
 * anyone else reads by `readSource`. */
function enumerationSource(target) {
  if (!inDraft(target) && !writeOverride && getObserver() !== null) {
    const base = read(getContainerNode(target));
    if (optHooks !== null && target.o) {
      const frame = optHooks.frame(target);
      if (frame !== NOT_PENDING) return frame;
    }
    return base;
  }
  const src = readSource(target);
  return optRead(target) ? optHooks.view(target, src) : src;
}
/** A read on an optimistic family that composes the lanes' values: not the
 * draft's (it composed already), not a derive's write-override read (the
 * derive composes on the truth). */
function optRead(target) {
  return optHooks !== null && target.o && !inDraft(target) && !writeOverride;
}
/** Pollution keys are never served from the prototype (core R30). */
const UNSAFE_KEYS = new Set(["__proto__", "prototype", "constructor"]);
// ---------------------------------------------------------------------------
// traps
/** A trap write: the target is told at the setter's exit, the key recorded
 * in this setter's `wk`. A setter's first write to a target still staged by
 * an earlier setter of the batch opens a set of its own (the notify visits
 * it, D-8) and chains the earlier one behind for the fold's union. */
function touch(target, key) {
  let wk = target.wk;
  if (!pendingNotify.has(target)) {
    pendingNotify.add(target);
    if (wk !== null && wk !== WK_ALL) {
      wk = new Set();
      wk.u = target.wk;
      target.wk = wk;
    }
  }
  if (wk !== WK_ALL) (target.wk ??= new Set()).add(key);
}
/** Dev: targets (and their keys) already warned about for a write outside
 * the setter — #4179's once-per-target+key memory. */
let warnedOutsideWrites;
/** Dev: set while the runtime itself writes through a proxy it knows the
 * trap ignores (`registerGraph`'s `_owner` stamp). */
let internalOutsideWrite = false;
/** Dev: a write, defineProperty or delete on a store proxy outside its
 * setter's draft is ignored (the trap returns `true` so strict-mode code
 * does not throw) — say so once per target+key instead of silently dropping
 * it (#4179). */
function warnWriteOutsideSetter(target, key, operation) {
  if (internalOutsideWrite) return;
  warnedOutsideWrites ??= new WeakMap();
  let keys = warnedOutsideWrites.get(target);
  if (keys === undefined) warnedOutsideWrites.set(target, (keys = new Set()));
  if (keys.has(key)) return;
  keys.add(key);
  const name = String(key);
  const access =
    typeof key === "string" && /^[A-Za-z_$][\w$]*$/.test(key)
      ? `.${key}`
      : typeof key === "string" && /^\d+$/.test(key)
        ? `[${key}]`
        : `[${typeof key === "string" ? JSON.stringify(key) : name}]`;
  const hint =
    operation === "deleteProperty"
      ? `setStore(s => { delete s${access} })`
      : `setStore(s => { s${access} = value })`;
  reportDiagnostic(
    emitDiagnostic({
      code: "STORE_WRITE_OUTSIDE_SETTER",
      kind: "write",
      severity: "warn",
      message:
        `[STORE_WRITE_OUTSIDE_SETTER] ${operation === "deleteProperty" ? "Deleting" : "Writing"} store property "${name}" outside its setter was ignored. ` +
        `Stores are read-only outside the setter's draft: use \`${hint}\`.`,
      nodeName: name,
      data: { operation, property: name }
    })
  );
}
/** #4808: a nested write through a child that is another store still
 * loading its first answer needs that store's contents as the draft's base,
 * and they do not exist yet (its seed is never observable, A25), so the
 * write is refused with the read's NotReadyError. Dev names the cause. */
function devWriteThroughLoading(target) {
  const name = String(target.pk ?? "");
  reportDiagnostic(
    emitDiagnostic({
      code: "STORE_WRITE_THROUGH_LOADING",
      kind: "write",
      severity: "warn",
      message:
        `[STORE_WRITE_THROUGH_LOADING] A store setter wrote inside "${name}", which is another ` +
        `store still loading its first answer. The write was dropped: when that store loads, its ` +
        `answer is what shows, as for a write to a loading store itself. Write after it loads, or ` +
        `replace the whole value: \`setStore(s => { s.${name} = value })\`.`,
      nodeName: name,
      data: { operation: "set", property: name }
    })
  );
}
const traps = {
  get(_, key, receiver) {
    const target = this;
    if (getObserver() === null) checkForeignRead();
    // One typeof gates every brand-symbol compare off the hot string path.
    if (typeof key !== "string") {
      if (key === $TARGET) return target;
      if (key === $PROXY) return receiver;
      if (key === $OWNER) return undefined; // ownership stamp: never a user key
      if (key === $RECORD) return undefined; // a store is no view (see `viewOf`)
      // refresh()/isPending resolve the projection computed through $REFRESH.
      if (key === $REFRESH) return target._fam?.node ?? undefined;
      if (key === $TRACK) {
        if (target._fam !== null) pullFamily(target);
        if (!inDraft(target) && getObserver() !== null) {
          read(getContainerNode(target));
          // Link the optimistic carrier the same way ownKeys/`enumerationSource`
          // does. Without this, a $TRACK-only reader never sees structural
          // guesses (push/splice/reorder) and is not re-run on revert (#5007).
          if (optHooks !== null && target.o && !writeOverride) optHooks.frame(target);
          // Structural chaining (§7b): a chained backing's $TRACK reads
          // through to the INNER store's container — structural
          // notifications land on the source's own node.
          const srcT = readSource(target);
          if (srcT[$TARGET] !== undefined) srcT[$TRACK];
        }
        return undefined;
      }
      // user symbols fall through to the generic path
    }
    // (The witness before the pull: a mark on an uninitialized derived
    // store is witnessed, then the pull throws — loading, declared pending.)
    if (verdict !== null && affectsHooks !== null && getObserver() === null)
      affectsHooks._witness(target, key);
    if (target._fam !== null) pullFamily(target);
    // S2: a key a live guess changed reads through the target's carrier,
    // tracked — the lane's seat, a verdict window's judgment of the guess
    // (A17/A24): the frame core serves this reader, indexed. The key's own
    // leaf is silent while the guess covers it (optimistic.ts).
    // S2: an optimistic family's target (one field test on a plain store's
    // read path; the arms are the optimistic hooks').
    // `optHooks !== null` first: a bundle without optimistic stores never
    // assigns the hook, and the minifier folds the whole arm away.
    // In the user's draft: a read of another action's live guess (#4788).
    if (optHooks !== null && target.o && !writeOverride) {
      if (inDraft(target)) optHooks.drafted(target, key);
      else {
        const v = optHooks.read(target, key);
        if (v !== NOT_PENDING) return v;
      }
    }
    // Hot inline case: existing PLAIN node (non-accessor) read outside any
    // draft — the dbmon/uibench effect re-read shape. Core's `read()` serves
    // it (linking a tracked reader; the hold rules either way). Skips
    // serveDataKey's frame, the FORCE compare (only accessor keys ever hold
    // the sentinel), and isWrappable for primitives. ONE node-map lookup
    // serves this block and the accessor probe below. Before the backing is
    // consulted: a deleted key's node serves `undefined` (notifyWrites). A
    // chained target's node is a subscription point and reads through below,
    // a lane value on it included: the derive's lane pass that wrote it
    // predates the chain, and serving it covered the inner store's truth
    // (#4762: `length` and a row of the old list beside the new list's
    // rows). An optimistic target's guesses are its carrier's (above).
    const node0 = target.n?.[key];
    if (writeScopes === null && !writeOverride) {
      const nodeH = node0;
      if (nodeH !== undefined && nodeH.acc !== true && !target.ch) {
        const nv = nodeValue(nodeH, undefined, target);
        if (nv === null || typeof nv !== "object") return nv;
        if (target.s) return serveShallow(target, key, nv);
        if (nodeH.pxv === nv) return nodeH.px;
        if (isWrappable(nv)) {
          const p = wrap(nv, target, key);
          nodeH.px = p;
          nodeH.pxv = nv;
          return p;
        }
        return nv;
      }
    }
    const src = readSource(target, key);
    // Overlay delete (#3044): a prototype overlay cannot shadow a delete, so
    // deleted keys are tracked aside and read as absent in the pending view.
    if (src === target.pb && target.del?.has(key)) {
      if (!inDraft(target) && getObserver() !== null) read(getNode(target, key));
      return undefined;
    }
    if (
      asyncTailFlights !== 0 &&
      untrackDepth === 0 &&
      !inDraft(target) &&
      typeof key === "string" &&
      key !== "then" &&
      getObserver() === null &&
      // Own data keys only: a pending backing inherits unrewritten keys from `v`.
      (hasOwn$1.call(src, key) || hasOwn$1.call(target.v, key))
    )
      // Once per store per computation: the holder is the root target, so a
      // row walk (`items.map(i => i.name)`) after an await reports the first
      // untracked key it touched, not one warning per row proxy.
      checkPostAwaitRead(target.n?.[key], storeRoot(target), undefined, key, false);
    // Dev strictRead: untracked store reads in labeled scopes (component
    // bodies, effect callbacks) warn — the value can never update the
    // reader. `then` is exempt: resolving a promise with a store proxy makes
    // the engine probe `.then` synchronously in the caller's scope — not a
    // read the user wrote.
    if (
      strictRead &&
      !inDraft(target) &&
      typeof key === "string" &&
      key !== "then" &&
      getObserver() === null &&
      strictReadActive()
    ) {
      warnStrictReadUntracked(strictRead, {
        nodeName: key,
        data: { strictRead, property: key, source: "store" }
      });
    }
    // Accessor keys serve through Reflect.get with the PROXY receiver
    // (R20/R29: internal reads track; the node is linked for shape-change
    // notification but its value is never served). Accessor-ness comes from
    // the node's cached flag; the first TRACKED read (which creates the
    // node) probes once — untracked node-less reads take the plain path,
    // where a raw-receiver getter still returns correct committed values.
    // Tracking suppression is PER-TARGET (inDraft), never global (#3037).
    let accProbe = -1;
    {
      let acc;
      if (node0 !== undefined) acc = node0.acc === true;
      else if (!inDraft(target) && getObserver() !== null) {
        acc = isAccessorKey(src, key);
        if (src === (target.pb ?? target.v)) accProbe = acc ? 1 : 0;
      } else acc = false;
      if (acc) {
        if (!inDraft(target) && getObserver() !== null)
          read(node0 ?? getNode(target, key, accProbe));
        const v = Reflect.get(src, key, receiver);
        if (target.s) return serveShallow(target, key, v);
        return isWrappable(v) ? draftServe(target, wrap(v, target, key)) : v;
      }
    }
    // Plain-data fast path: no descriptor allocation per read. Inherited
    // pollution keys are never served (core R30) — checked before the
    // proto-function branch can leak `constructor`. An inherited
    // `constructor` is withheld from drafts only: a plain read answers the
    // source's class (`s.o.constructor === Obj`, #4670). Overlay pending backings
    // chain to the committed backing, so "own in the view" means own on
    // either layer — a genuine prototype method is one that is own on
    // NEITHER.
    const viewOvl = target.ovl && src === target.pb;
    if (
      (key === "constructor"
        ? writeOverride || inDraft(target)
        : key === "__proto__" || key === "prototype") &&
      !hasOwn$1.call(src, key) &&
      !(viewOvl && hasOwn$1.call(target.v, key))
    )
      return undefined;
    let v = src[key];
    if (
      v === undefined
        ? !hasOwn$1.call(src, key) && !(viewOvl && hasOwn$1.call(target.v, key))
        : false
    ) {
      // Inherited: prototype getters/methods run with the proxy receiver.
      v = Reflect.get(src, key, receiver);
      if (typeof v === "function") return v; // proto methods untracked
      // Read-through (§7b): the inner store answers an absent key too — the
      // outer's node is a subscription point unless it carries a guess.
      if (target.ch && src === target.v) return serveDataKey(target, key, v, src, node0, accProbe);
      // Reading a currently-absent own key subscribes to it (R12) — for any
      // target OUTSIDE its own draft scope, even mid-setter (#3037, above).
      if (v === undefined && !inDraft(target)) {
        // (`nodeValue` reads — and links — the node it creates.)
        if (getObserver() !== null) getNode(target, key, accProbe);
        const node = target.n?.[key];
        if (node) {
          const nv = nodeValue(node, undefined, target);
          if (target.s) return serveShallow(target, key, nv);
          return isWrappable(nv) ? draftServe(target, wrap(nv, target, key)) : nv;
        }
      }
      if (target.s) return serveShallow(target, key, v);
      return isWrappable(v) ? draftServe(target, wrap(v, target, key)) : v;
    }
    if (
      typeof v === "function" &&
      !hasOwn$1.call(src, key) &&
      !(viewOvl && hasOwn$1.call(target.v, key))
    )
      return v; // proto method
    return serveDataKey(target, key, v, src, node0, accProbe);
  },
  has(_, key) {
    const target = this;
    if (key === $TARGET || key === $PROXY || key === $TRACK) return true;
    if (key === $OWNER || key === $RECORD) return false;
    // (The witness before the pull: a mark on an uninitialized derived
    // store is witnessed, then the pull throws — loading, declared pending.)
    if (verdict !== null && affectsHooks !== null && getObserver() === null)
      affectsHooks._witness(target, key);
    if (target._fam !== null) pullFamily(target);
    // In the user's draft: presence a live guess changed (#4812).
    if (optHooks !== null && target.o && !writeOverride && inDraft(target))
      optHooks.drafted(target, key, true);
    // S2: presence of a key a live guess changed is the carrier frame's.
    if (optRead(target)) {
      const guessed = optHooks.has(target, key);
      if (guessed !== undefined) return guessed;
    }
    const src = readSource(target, key);
    // A tracked reader's presence node answers (born from the two frames;
    // core decides which it sees); everyone else reads the backing. A
    // chained target's node is a subscription point (§7b): the inner store
    // answers through the backing — unless the outer node carries a guess.
    if (!inDraft(target) && !writeOverride) {
      if (getObserver() !== null) {
        const node = getHasNode(target, key);
        const nv = read(node);
        if (!target.ch || src !== target.v) return !!nv;
        const present = key in src;
        node._value = present; // the link follows the inner store (S4)
        return present;
      }
    }
    return key in src && !(src === target.pb && target.del?.has(key));
  },
  ownKeys() {
    const target = this;
    if (verdict !== null && affectsHooks !== null && getObserver() === null)
      affectsHooks._witness(target, undefined);
    if (target._fam !== null) pullFamily(target);
    if (optHooks !== null && target.o && !writeOverride && inDraft(target))
      optHooks.drafted(target, undefined, true);
    return visibleKeys(target, enumerationSource(target));
  },
  getOwnPropertyDescriptor(shell, key) {
    const target = this;
    if (key === $OWNER || key === $RECORD) return undefined;
    const obs = getObserver();
    if (verdict !== null && affectsHooks !== null && obs === null)
      affectsHooks._witness(target, key);
    if (target._fam !== null) pullFamily(target);
    // An enumerator (spread, Object.entries) already holds the container
    // node and reads its frame; a descriptor read on its own tracks
    // presence (R13).
    const plain = obs !== null && !inDraft(target) && !writeOverride;
    const enumerating = plain && observerHoldsContainer(target, obs);
    // S2: a key a live guess changed is described from the carrier's frame —
    // before the presence node answers (silent while the guess covers it).
    if (!enumerating && optRead(target)) {
      const guessed = optHooks.descriptor(target, key, Array.isArray(shell));
      if (guessed !== undefined) return guessed === null ? undefined : guessed;
    }
    let src;
    if (enumerating) src = enumerationSource(target);
    else if (plain) {
      // The presence node's frame (core decides which) is the one described
      // (a chained target's inner store answers through the backing, §7b);
      // the descriptor itself comes from the backing the container's frame
      // serves this reader (`readSource`: a held container's changed key —
      // enumerability, accessor-ness — reads committed for a stale reader,
      // the staging for a member; #3706 contrast).
      const node = getHasNode(target, key);
      const present = read(node);
      src = readSource(target, key, true);
      if (!present && (!target.ch || !(key in src))) return undefined;
    } else src = readSource(target, key, true);
    let desc;
    desc ??= visibleDescriptor(target, src, key);
    if (desc === undefined) return undefined;
    // The shell's `length` is writable and non-configurable, and a proxy must
    // report it that way (a frozen source array's is non-writable).
    if (key === "length" && Array.isArray(shell)) desc.writable = true;
    else desc.configurable = true;
    return desc;
  },
  set(_, key, value) {
    const target = this;
    const draft = inDraft(target);
    if (!draft && !writeOverride) {
      warnWriteOutsideSetter(target, key, "set");
      return true;
    }
    if (key === "__proto__") return true; // pollution guard (core R30)
    // A derived store's draft keeps a proxy of another store (a plain store
    // or another family; its own drafts still unwrap), and the key chains to
    // it (§7b read-through; CS-R3, CS-R5) as the return form's backing does.
    // Unwrapped, the family re-wrapped a raw that the source store's next
    // write left behind (#4845). A plain store's raw dedupes to the source's
    // own node (CS-R2), so plain stores unwrap as before.
    const uv =
      target.s || (target._fam && value?.[$TARGET]?._fam !== target._fam)
        ? value
        : unwrapValue(value);
    const pb = ensurePB(target);
    // #4146 (R1: node existence is unobservable; #3706: the hold's unit is
    // the key): a user setter writing a key a HELD container's staging left
    // unchanged would write it into the held staging — read as the hold's
    // until its landing, while the same key with a leaf (a live reader)
    // publishes at the flush. The key gets its leaf first, born from the two
    // frames (`bornStaged`: a key the hold left unchanged — or added — is
    // unstaged and held by no one; a key the hold changed is born held, A34
    // (1)), and the write stages on it (A28; a write by the holder joins it,
    // A34) — as it would with a reader's leaf. Kept while the container is
    // held (`deferredReleases`).
    if (userWrite && target.k?._config & CONFIG_HELD) deferredReleases.add(getNode(target, key));
    if (Array.isArray(pb)) {
      touch(target, key);
      if (key === "length") target.wk = WK_ALL;
      else touch(target, "length");
    } else {
      touch(target, key);
      if (!(key in pb)) target.kc++;
    }
    const unsafe = UNSAFE_KEYS.has(key);
    if (unsafe || (target.ovl && target.sc !== 2 && !hasOwn$1.call(pb, key))) {
      Object.defineProperty(pb, key, {
        value: uv,
        writable: true,
        enumerable: true,
        configurable: true
      });
    } else pb[key] = uv;
    target.del?.delete(key);
    if (unsafe) return true;
    if (target.s && uv !== null && typeof uv === "object") markRawOne(uv);
    if (!draft) notifyWrites(target); // a write-override write
    return true;
  },
  defineProperty(_, key, desc) {
    const target = this;
    const draft = inDraft(target);
    if (!draft && !writeOverride) {
      warnWriteOutsideSetter(target, key, "defineProperty");
      return true;
    }
    if (key === "__proto__") return true;
    if (desc.get || desc.set) target.a = true;
    if ("value" in desc) desc = { ...desc, value: unwrapValue(desc.value) };
    const pb = ensurePB(target);
    if (target.a || !(desc.enumerable && desc.writable && desc.configurable)) target.sc = 1;
    touch(target, key);
    Object.defineProperty(pb, key, desc);
    target.del?.delete(key);
    if (!draft) notifyWrites(target); // a write-override write
    return true;
  },
  deleteProperty(_, key) {
    const target = this;
    const draft = inDraft(target);
    if (!draft && !writeOverride) {
      warnWriteOutsideSetter(target, key, "deleteProperty");
      return true;
    }
    const pb = ensurePB(target);
    touch(target, key);
    // The key count (`kc`, the overlay gate and the rebuild cap) follows a
    // delete the way it follows an add: an own key of the draft leaving — a
    // clone's key, or an overlay's own add — is one fewer now. An overlay's
    // override of a committed key is the exception: the committed key's
    // deletion is recorded in `del` and counted at the rebuild, as before.
    // Without the decrement the natural "delete the old key, set the new
    // one" shape drifted the count up by one per write until a three-key
    // object crossed the overlay threshold and paid a rebuild on every
    // commit after.
    const committed = target.ovl && hasOwn$1.call(target.v, key);
    if (hasOwn$1.call(pb, key)) {
      delete pb[key];
      committed || target.kc--;
    }
    if (committed) (target.del ??= new Set()).add(key);
    if (!draft) notifyWrites(target); // a write-override write
    return true;
  },
  // The shell is empty: a store cannot be frozen or re-prototyped. It stays
  // extensible, so the source's prototype may be reported (#4670: instanceof).
  getPrototypeOf() {
    return protoOf(this.v);
  },
  preventExtensions: () => false,
  setPrototypeOf: () => false
};
TargetShape.prototype = traps;
/** Is `obs` subscribed to the container BY THIS PASS (its enumerator holds
 * it)? A link left from a previous pass is stale — the reader must subscribe
 * to the key's presence node or a later add never re-runs it. */
function observerHoldsContainer(target, obs) {
  const k = target.k;
  if (k === null) return false;
  const l = k._subsTail;
  return (
    l !== null &&
    l._sub === obs &&
    (!(obs._flags & REACTIVE_RECOMPUTING_DEPS) || l._gen === obs._depGen)
  );
}
// ---------------------------------------------------------------------------
// setter / createStore
const ASYNC_STORE_SETTER_MESSAGE =
  "[ASYNC_STORE_SETTER] Store setter callback returned a Promise. A store setter is a synchronous transaction: " +
  "the draft closes when the callback returns, so writes after an `await` are lost. " +
  "Move the await into an action() and call the setter from there.";
/** A store setter inside an owned scope (a memo body) is the owned-scope
 * write violation — the user guard (`mapArray`'s pattern: the node-level
 * writes carry `ownedWrite`; the setter asks). */
function devGuardStoreSetterWrite() {
  if (
    context &&
    !(context._config & CONFIG_CHILDREN_FORBIDDEN) &&
    !disposalDepth &&
    !inRefCallback()
  ) {
    emitDiagnostic({
      code: "REACTIVE_WRITE_IN_OWNED_SCOPE",
      kind: "write",
      severity: "error",
      message: REACTIVE_WRITE_IN_OWNED_SCOPE_SIGNAL_MESSAGE,
      ownerId: context.id,
      ownerName: context._name,
      data: { operation: "setStore" }
    });
    throw new Error(ownedScopeWriteMessage(context));
  }
}
/** The callback's return has one meaning — a replacement root to adopt — and
 * a thenable can never be that: it is the signature of `setStore(async d =>
 * …)`, where only the writes before the first `await` were in the
 * transaction. Setters are synchronous; async orchestration is `action()`'s. */
function devGuardStoreSetterResult(result) {
  if (result == null) return;
  if (
    (typeof result === "object" || typeof result === "function") &&
    typeof result.then === "function"
  ) {
    emitDiagnostic({
      code: "ASYNC_STORE_SETTER",
      kind: "write",
      severity: "error",
      message: ASYNC_STORE_SETTER_MESSAGE,
      ownerId: context?.id,
      ownerName: context?._name,
      data: { operation: "setStore" }
    });
    throw new Error(ASYNC_STORE_SETTER_MESSAGE);
  }
}
function storeSetter(proxy, fn, guard = true) {
  if (writeHook !== null) writeHook(proxy);
  if (guard) devGuardStoreSetterWrite();
  const target = proxy[$TARGET];
  const prevScopes = writeScopes;
  writeScopes = new Set();
  writeScopes.add(scopeKey(target));
  const prevUser = userWrite;
  userWrite = guard;
  // An outermost setter's exit notifies the targets it wrote. A
  // write-override write (a derive's continuation after an `await`, no
  // setter open) is notified at once by its trap but stays listed (its keys
  // share one written set while the continuation writes): dropped here, or
  // this setter's exit would notify the derive's staging as its own — a
  // user's, on an optimistic family, as guesses (#4484). Read before the
  // clear: the list is empty here but for those, and clearing an empty set
  // allocates.
  if (writing === 0 && pendingNotify.size) pendingNotify.clear();
  writing++;
  let result;
  try {
    // No untrack: the writing flag already disables store-node linking
    // (draft reads never self-track, proj R2), while EXTERNAL reads (signals
    // inside a projection derive) must keep tracking — they are the derive's
    // dependencies.
    result = fn(proxy);
    // A returned replacement on an optimistic family is a user's edit, and
    // its slot is the point of write (S-59): written into the draft as the
    // keyed diff — a same-key row recurses into its own draft — so it makes
    // the writes the field edit would, and the exit below notifies them.
    if (
      optHooks !== null &&
      guard &&
      target.o &&
      !writeOverride &&
      result !== undefined &&
      result !== proxy &&
      isWrappable(result)
    ) {
      optHooks.reconcile(proxy, unwrapValue(result), target._fam?.key ?? null);
      result = undefined;
    }
  } finally {
    writing--;
    writeScopes = prevScopes;
    // Outermost setter exit: emit write-time notifications (one node write
    // per changed observed key) so holds and lanes engage now.
    if (writing === 0 && pendingNotify.size) {
      if (draftReadTx !== null) joinFuture(draftReadTx);
      const touched = [...pendingNotify];
      pendingNotify.clear();
      for (const t of touched) notifyWrites(t);
      // A direct commit (a projection's creation run) folds at the setter's
      // exit, unqueued: the backing is the pre-batch one (nothing swapped).
      if (directCommit) for (const t of touched) if (t.pb !== null) foldTarget(t, t.v);
    }
    userWrite = prevUser;
    if (writing === 0) draftReadTx = null;
  }
  // After the sync writes have notified (they were real, like an effect's
  // side effects before its invalid-cleanup throw) and before adoption.
  if (guard) devGuardStoreSetterResult(result);
  if (result !== undefined && result !== proxy && isWrappable(result)) {
    // A returned replacement off the optimistic path is adopted as the
    // incoming truth.
    adoptPB(target, unwrapValue(result));
  }
  // A34 (3), #3612 — the derived store's twin of `setMemo`: a user write to
  // a derivation another transaction holds is nobody's proposal; it becomes
  // the draft's prior state and the hold re-derives over it (the writes
  // joined the hold through `setSignal`; the derive re-runs under it).
  if (guard && heldDerivationHit) {
    heldDerivationHit = false;
    const fw = target._fam.node;
    if (!(fw._flags & REACTIVE_DIRTY)) {
      fw._flags = (fw._flags & ~REACTIVE_CHECK) | REACTIVE_DIRTY;
      insertIntoHeap(fw, dirtyQueue);
      schedule();
    }
  }
}
function createStore$1(initialValue, options) {
  const shallow = options?.shallow === true;
  if (shallow && true) {
    // Never both deep-wrapped and raw (R41/R44): a value already tracked as
    // a DEEP store cannot be ingested shallow.
    const existing = lookupTarget(initialValue, null);
    if (existing !== undefined && !existing.s)
      throw new Error("createStore({ shallow }): value is already tracked as a deep store");
    if (initialValue[$TARGET])
      throw new Error("createStore({ shallow }): value is already a store proxy");
  }
  const proxy = wrap(initialValue);
  // Dependency edges (store/edges.ts): the root is named like a plain signal.
  if (signalHook !== null) signalHook(proxy[$TARGET]);
  if (shallow) {
    proxy[$TARGET].s = true;
    markRawIngest(initialValue);
  }
  {
    const owner = getOwner();
    // Dev-tier graph registration (owner signal lists, onGraph); the
    // `_owner` write itself never reaches the proxy, see storeOwners — an
    // internal ignored write, not the user's (#4179's warning skips it).
    internalOutsideWrite = true;
    registerGraph(proxy, owner);
    internalOutsideWrite = false;
    // Only once the engine is installed, like the node stamping it feeds: a
    // WeakMap.set per fresh store is a growing ephemeron table and,
    // disabled, buys nothing.
    if (attrHooks !== null) {
      storeOwners.set(proxy[$TARGET], owner);
      nameStore(proxy, options?.name, options?.loc);
    }
  }
  const setter = fn => storeSetter(proxy, fn);
  return [proxy, setter];
}
/** The list driver's slot view of a shallow store (types.ts `shallowView`):
 * one `readSource` for the container instead of a trap per slot. A shallow
 * store's slots hold raw records, so the backing the container read serves
 * is what each slot read would — except where a slot's own leaf can answer
 * differently: an optimistic family's guesses, a chained backing, accessor
 * keys, or a read inside a draft or write scope. Those keep the trap. */
installShallowView(t => {
  // Any open write scope keeps the trap (it covers `inDraft`).
  if (t?.s && !(t.ch || t.a || writeScopes || writeOverride || t._fam?.opt))
    return (t._fam && pullFamily(t), readSource(t));
});
/** True when `proxy` is a SHALLOW store (children served verbatim, slots
 * replaced by reference — #2932). The list driver uses this to choose the
 * slot-patch channel (collected row bodies) over per-record registration. */
function storeIsShallow(proxy) {
  const t = proxy?.[$TARGET];
  return t?.s === true;
}
/** True when `proxy` belongs to a projection family (S3). */
function storeHasFamily(proxy) {
  const t = proxy?.[$TARGET];
  return t?._fam != null;
}
/** True when `proxy` belongs to an OPTIMISTIC family (S4). */
function storeHasOptimisticFamily(proxy) {
  const t = proxy?.[$TARGET];
  return t?.o === true;
}
// ---------------------------------------------------------------------------
// enumeration views (one visibility rule for the traps AND the deep walk)
function visibleKeys(target, src) {
  let keys;
  if (target.ovl && src === target.pb) {
    keys = Reflect.ownKeys(target.v);
    const del = target.del;
    if (del?.size) keys = keys.filter(key => !del.has(key));
    for (const key of Reflect.ownKeys(src)) {
      if (!hasOwn$1.call(target.v, key)) keys.push(key);
    }
  } else keys = Reflect.ownKeys(src);
  // The ownership stamp is the last symbol on an owned backing: hide it.
  for (let i = keys.length - 1; i >= 0 && typeof keys[i] === "symbol"; i--) {
    if (keys[i] === $OWNER) {
      keys.splice(i, 1);
      break;
    }
  }
  return keys;
}
function visibleDescriptor(target, src, key) {
  let desc = Object.getOwnPropertyDescriptor(src, key);
  if (target.ovl && src === target.pb) {
    if (target.del?.has(key)) return undefined;
    if (desc === undefined) desc = Object.getOwnPropertyDescriptor(target.v, key);
  }
  return desc;
}
// ---------------------------------------------------------------------------
// deep / snapshot
/** Tracking snapshot: subscribes every reachable record's container and deep
 * witness (one node per record, not one per path), then snapshots. */
function deep$1(value) {
  const t0 = value?.[$TARGET];
  if (t0?.px !== value) return value;
  const visited = new Set();
  const childTarget = (t, child, key) => {
    let ct = lookupTarget(child, t._fam);
    if (ct === undefined) {
      if (!isWrappable(child)) return undefined;
      wrap(child, t, key);
      ct = lookupTarget(child, t._fam) ?? child[$TARGET];
    }
    return ct;
  };
  const walkT = t => {
    if (t._fam !== null) pullFamily(t);
    const src = enumerationSource(t);
    if (visited.has(src)) return;
    visited.add(src);
    read(getDeepNode(t));
    // Through a chain (#3323): every inner record's container AND deep
    // witness — base writes bump the inner witnesses.
    for (let it = t; it.ch;) {
      it = it.v[$TARGET];
      read(getContainerNode(it));
      read(getDeepNode(it));
    }
    for (const key of visibleKeys(t, src)) {
      const desc = visibleDescriptor(t, src, key);
      if (desc === undefined) continue;
      if (desc.get || desc.set) {
        t.a = true;
        continue; // accessors track through their own reads when invoked
      }
      let child = desc.value;
      if (child === null || typeof child !== "object") continue;
      if (t.ch && child[$TARGET] === undefined) child = resolveChainedRaw(t, key, child);
      const ct = childTarget(t, child, key);
      if (ct === undefined) continue; // raw-marked: leaf by contract
      walkT(ct);
    }
  };
  // One pass where the tracked view is the snapshot's backing; else two walks.
  const seen = new Map();
  const fast = (t, raw) => {
    if (t._fam !== null || t.ch || t.ovl || (raw !== t.px && lookupTarget(raw, null) !== t)) return;
    const src = enumerationSource(t);
    if (src !== snapshotBacking(t) || visited.has(src) || seen.has(src) || !isWrappable(src))
      return;
    // snapshotWalk's resolution must stop at `src` too.
    if (src !== raw) {
      const st = lookupTarget(src, null);
      if (st !== undefined && st !== t) return;
    }
    visited.add(src);
    read(getDeepNode(t));
    return src;
  };
  const step = (t, cv, key, trackOnly) => {
    const ct = childTarget(t, cv, key);
    const src = ct && !trackOnly ? fast(ct, cv) : undefined;
    if (src) return snapshotRecord(src, seen, null, ct, step);
    if (ct) walkT(ct);
    if (!trackOnly) return snapshotWalk(cv, seen, null);
  };
  try {
    const src = fast(t0, value);
    if (src) return snapshotRecord(src, seen, null, t0, step);
  } catch (e) {
    // A snapshot read threw mid-pass: subscribe the rest, as the two walks did.
    visited.clear();
    walkT(t0);
    throw e;
  }
  walkT(t0);
  return snapshotWalk(value, seen, t0._fam);
}
/** Non-tracking snapshot: source identity for unowned subtrees (zero copy),
 * a copy for owned ones (they are by definition modified relative to the
 * source). Sees the pending backing (R27). */
function snapshot$1(value) {
  const t = value?.[$TARGET];
  return snapshotWalk(value, new Map(), t?._fam ?? null);
}
/** R27: a snapshot sees the batch's pending backing — a held one (another
 * transaction's future) only from its own draft, or from a pass the
 * container node serves the future (its landing flush, a memo joining). */
function snapshotBacking(t) {
  let backing = t.v;
  const pb = t.pb;
  if (pb !== null) {
    const k = t.k;
    if (inDraft(t) || writeOverride || k === null || !(k._config & CONFIG_HELD)) backing = pb;
    else if (stagingReader() !== null) backing = untrack(() => read(k));
    // materializePB is a no-op (returns pb) without an overlay.
    if (backing === pb) backing = materializePB(t);
  }
  return optRead(t) ? optHooks.view(t, backing) : backing;
}
function snapshotWalk(value, seen, fam) {
  if (value === null || typeof value !== "object") return value;
  let src = value;
  for (let entry = true; ; entry = false) {
    const viaProxy = src?.[$TARGET]?.v !== undefined;
    let t = viaProxy ? src[$TARGET] : undefined;
    if (t === undefined && fam !== null) t = lookupTarget(src, fam);
    if (t === undefined) t = lookupTarget(src, null);
    if (t === undefined) break;
    if (entry && !viaProxy && fam !== null && t._fam !== fam) {
      const outer = fam.map.get(t.px);
      if (outer !== undefined) t = outer;
    }
    if (t._fam !== null) fam = t._fam;
    const backing = snapshotBacking(t);
    if (backing === src) break;
    src = backing;
  }
  if (!isWrappable(src)) return src;
  const cached = seen.get(src);
  return cached !== undefined ? cached : snapshotRecord(src, seen, fam, null, null);
}
/** `t`/`step` set: `deep()`'s one pass, which also tracks `t`'s children. */
function snapshotRecord(src, seen, fam, t, step) {
  if (isOwned(src)) {
    const isArr = Array.isArray(src);
    const copy = isArr ? [] : Object.create(protoOf(src));
    seen.set(src, copy);
    for (const key of Reflect.ownKeys(src)) {
      if ((isArr && key === "length") || key === $OWNER) continue;
      const desc = Object.getOwnPropertyDescriptor(src, key);
      const cv = desc.value;
      const obj = cv !== null && typeof cv === "object";
      if (typeof key === "symbol" && !desc.enumerable) {
        if (step && obj) step(t, cv, key, true);
        continue;
      }
      if (desc.get || desc.set) {
        if (t) t.a = true;
        Object.defineProperty(copy, key, desc);
        continue;
      }
      const walked = obj ? (step ? step(t, cv, key) : snapshotWalk(cv, seen, fam)) : cv;
      if (desc.enumerable && desc.writable && desc.configurable) copy[key] = walked;
      else Object.defineProperty(copy, key, { ...desc, value: walked });
    }
    if (isArr && copy.length !== src.length) copy.length = src.length;
    return copy;
  }
  seen.set(src, src);
  let copy = null;
  for (const key of Reflect.ownKeys(src)) {
    const desc = Object.getOwnPropertyDescriptor(src, key);
    if (!desc) continue;
    if (desc.get || desc.set) {
      if (t) t.a = true;
      continue;
    }
    const cv = desc.value;
    if (cv === null || typeof cv !== "object") continue;
    const walked = step ? step(t, cv, key) : snapshotWalk(cv, seen, fam);
    if (walked !== cv) {
      if (copy === null) {
        copy = Array.isArray(src)
          ? [...src]
          : Object.create(protoOf(src), Object.getOwnPropertyDescriptors(src));
        seen.set(src, copy);
      }
      copy[key] = walked;
    }
  }
  return copy ?? src;
}
// Observe: what the attribution engine needs of a store to judge a
// `storeWrite` (its spread-copy census). Registered here so the engine never
// imports this module — an app without stores does not load it for turning
// attribution on.
{
  registerStoreTools({
    pathOf(target) {
      let path = "";
      for (let cur = target; cur !== null; cur = cur.u)
        path = cur.pk === null ? "store" + path : "." + String(cur.pk) + path;
      return path;
    },
    isShallow: target => target.s,
    deleted: target => target.del,
    rootOwner: target => storeRootOwner(target),
    ownerKey: $OWNER,
    unwrapValue,
    targetsEqual,
    isRawValue,
    isWrappable
  });
}
// Devtools `DEV.setSignal` for store property slots (#4679): go through the
// store setter so backing state and `onStoreNodeUpdate` stay consistent.
{
  registerDevStoreSlotWriter((node, value) => {
    const host = node._host;
    const key = node._key;
    if (!host?.px || key === $TRACK) {
      setSignal(node, value);
      return;
    }
    storeSetter(host.px, draft => {
      draft[key] = value;
    });
  });
}

const DELETE = Symbol("STORE_PATH_DELETE");
function isPrototypePollutionKey(part) {
  return part === "__proto__" || part === "constructor" || part === "prototype";
}
function updatePath(current, args, i = 0) {
  let part,
    prev = current;
  if (i < args.length - 1) {
    part = args[i];
    const partType = typeof part;
    const isArray = Array.isArray(current);
    if (partType === "string" && isPrototypePollutionKey(part)) return;
    if (Array.isArray(part)) {
      for (let j = 0; j < part.length; j++) {
        args[i] = part[j];
        updatePath(current, args, i);
      }
      args[i] = part;
      return;
    } else if (isArray && partType === "function") {
      for (let j = 0; j < current.length; j++) {
        if (part(current[j], j)) {
          args[i] = j;
          updatePath(current, args, i);
        }
      }
      args[i] = part;
      return;
    } else if (isArray && partType === "object") {
      const { from = 0, to = current.length - 1, by = 1 } = part;
      for (let j = from; j <= to; j += by) {
        args[i] = j;
        updatePath(current, args, i);
      }
      args[i] = part;
      return;
    } else if (i < args.length - 2) {
      updatePath(current[part], args, i + 1);
      return;
    }
    prev = current[part];
  }
  let value = args[args.length - 1];
  if (typeof value === "function") {
    value = value(prev);
    if (value === prev) return;
  }
  if (part === undefined && value == undefined) return;
  if (value === DELETE) {
    delete current[part];
  } else if (
    part === undefined ||
    (isWrappable(prev) && isWrappable(value) && !Array.isArray(value))
  ) {
    const target = part !== undefined ? current[part] : current;
    const keys = ownEnumerableKeys(value);
    for (let i = 0; i < keys.length; i++) {
      const key = keys[i];
      if (typeof key === "string" && isPrototypePollutionKey(key)) continue;
      const desc = Object.getOwnPropertyDescriptor(value, key);
      if (desc.get || desc.set) Object.defineProperty(target, key, desc);
      else target[key] = desc.value;
    }
  } else {
    current[part] = value;
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
 */
const storePath = /* @__PURE__ */ Object.assign(
  function storePath(...args) {
    return state => {
      updatePath(state, args);
    };
  },
  { DELETE }
);

/**
 * Store — reconcile: the immutable-diff adoption channel (INTERNALS §3).
 *
 * Reconcile never merge-writes into backing objects: it adopts `incoming`
 * as the backing at every proxied level (an internal pointer swap at the
 * fold — no clones, no user-object mutation), value-notifies only where
 * nodes exist, and descends into a changed child pair only where a target
 * with nodes or the sticky descendants flag exists below (§6d: diff the
 * listened path, not every cell). Reference-equal unowned pairs skip
 * outright (an owned backing is setter-diverged and must diff).
 *
 * Keyed arrays: the aligned prefix is walked positionally; from the first
 * misalignment rows match by key (surviving rows keep their proxy
 * identity), unkeyed items fall back to position.
 */
function reconcileState(value, state, key, replace = false) {
  if (state == null) throw new Error("Cannot reconcile null or undefined state");
  const t = state[$TARGET];
  if (t?.px !== state) throw new Error("reconcile target is not a store proxy");
  const keyFn = key === null ? null : typeof key === "string" ? item => item?.[key] : key;
  // A user's reconcile on an optimistic family (S4): not an adoption of
  // truth but an optimistic edit — the keyed diff is written into the draft
  // (a matched row keeps its proxy, its changed leaves become its guesses),
  // and the setter's exit turns the draft into guesses like any.
  if (optHooks !== null && t.o && userWriting()) {
    optHooks.reconcile(state, unwrapValue(value), keyFn);
    return;
  }
  // Replace-mode root handed another store's proxy: chain to it (§7b).
  if (replace && value !== state && value?.[$TARGET] !== undefined) {
    const prev = t.pb ?? t.v;
    if (prev === value) return; // already chained to this store
    adoptPB(t, value);
    return;
  }
  const incoming = unwrapValue(value);
  if (keyFn) {
    const prev = t.pb ?? t.v;
    const eq = keyFn(prev);
    if (eq !== undefined && !sameKey(keyFn(incoming), eq)) {
      if (!replace) throw new Error("Cannot reconcile states with different identity");
      // A different entity at the root: the old backing is disowned (wraps
      // fresh if re-handed) and the new one adopted whole.
      if (isOwned(prev)) delete prev[$OWNER];
      else (t._fam?.map ?? storeLookup).delete(prev);
      adoptPB(t, incoming);
      return;
    }
  }
  applyAdopt(t, incoming, keyFn, replace);
}
/** Adopt `incoming` at `t` and notify its nodes — one leaf write per
 * changed observed key, the structural nodes once — descending into
 * changed child pairs that are proxied below. */
function applyAdopt(t, incoming, keyFn, proj = false) {
  // The diff base is the view the nodes were last told (#3296). A nested
  // overlay draft (`Object.create(v)`, #3044) reads a key the draft deleted
  // as present through its prototype; materialized first — `adoptPB` does
  // so anyway, after the base was taken — so a deletion the draft made is
  // a deletion here (#3743: the presence and value diffs both read `old`).
  const old = t.ovl ? materializePB(t) : (t.pb ?? t.v);
  if (incoming === old && !isOwned(old)) return;
  // Q-D: a landing on an optimistic family is reconciled against the VIEW
  // — key matching from the arrangement the guesses made (R28), so a row
  // the guess added keeps its proxy when the server returns it.
  // A returned landing pairs its new objects with the row targets first, as
  // the derive's draft does (store.ts `notifyWrites`): a guessed row the view
  // no longer has (an answer that dropped it) keeps its target when the
  // answer brings its key back (Q-D, F-11, #4907). A row another store lent
  // is in the view already, and keeps its slot this store's (#4918).
  const prevView =
    optHooks !== null && t.o ? (optHooks.pair(t, incoming, true), optHooks.view(t, old)) : old;
  const nextArr = Array.isArray(incoming);
  const shallow = t.s === true;
  adoptPB(t, incoming, false);
  if (shallow) markRawIngest(incoming);
  // The deep witness is bumped once per adoption (none to bump: done).
  let bumped = t.dk === null;
  // A live chained base (a store proxy, §7b) reads the inner store's
  // current frame, not what the nodes were last told — nothing to diff
  // against: the fold writes every node (#4361).
  if (Array.isArray(prevView) !== nextArr || old[$TARGET]) {
    notifyFold(t, old, incoming);
    return;
  }
  if (nextArr) {
    const nodes = t.n;
    let nodesHit = 0;
    if (keyFn && !shallow) {
      const plen = prevView.length;
      const nlen = incoming.length;
      let i = 0;
      // Aligned prefix: same row (by reference or key) at the same index.
      for (const end = Math.min(plen, nlen); i < end; i++) {
        const nv = incoming[i];
        const pvRaw = prevView[i];
        if (pvRaw !== nv && !sameRow(pvRaw, nv, keyFn)) break; // misaligned: fall to the keyed remainder below
        if (nv !== null && typeof nv === "object" && (pvRaw !== nv || isOwned(nv)))
          descend(unwrapValue(pvRaw), nv, keyFn, proj, t, i);
        if (
          deepUsed &&
          !bumped &&
          !(nv !== null && typeof nv === "object" ? targetsEqual(pvRaw, nv) : isEqual(pvRaw, nv))
        ) {
          bumpDeep(t);
          bumped = true;
        }
        if (nodes !== null) {
          const node = nodes[i];
          if (node !== undefined) {
            nodesHit++;
            notifyKeyValue(node, i, old[i], nv, old, incoming);
          }
        }
      }
      if (deepUsed && !bumped && i < incoming.length) bumpDeep(t);
      const structStart = i; // misalignment point (== nlen on aligned ticks)
      let prevByKey = null;
      for (; i < incoming.length; i++) {
        const nv = incoming[i];
        if (nv !== null && typeof nv === "object") {
          const nk = keyFn(nv);
          let pv;
          if (nk !== undefined) {
            if (prevByKey === null) {
              prevByKey = new Map();
              for (let j = structStart; j < prevView.length; j++) {
                const p = unwrapValue(prevView[j]);
                if (p !== null && typeof p === "object") {
                  const pk = keyFn(p);
                  if (pk === undefined) continue;
                  const existing = prevByKey.get(pk);
                  if (existing === undefined) prevByKey.set(pk, j);
                  else if (Array.isArray(existing)) existing.push(j);
                  else prevByKey.set(pk, [existing, j]);
                }
              }
            }
            const m = prevByKey.get(nk);
            if (m === undefined) pv = undefined;
            else if (Array.isArray(m)) {
              pv = unwrapValue(prevView[m.shift()]);
              if (m.length === 1) prevByKey.set(nk, m[0]);
            } else {
              pv = unwrapValue(prevView[m]);
              prevByKey.delete(nk);
            }
          } else {
            pv = unwrapValue(prevView[i]); // keyless item: positional fallback
          }
          descend(pv, nv, keyFn, proj, t, i);
        }
        if (nodes !== null) {
          const node = nodes[i];
          if (node !== undefined) {
            nodesHit++;
            notifyKeyDiff(node, i, old, incoming, false);
          }
        }
      }
    } else {
      const dlen = Math.min(prevView.length, incoming.length);
      const nlen = incoming.length;
      for (let i = 0; i < nlen; i++) {
        const nvP = incoming[i];
        if (!shallow && i < dlen && nvP !== null && typeof nvP === "object")
          descend(unwrapValue(prevView[i]), nvP, keyFn, proj, t, i);
        if (
          deepUsed &&
          !bumped &&
          !(nvP !== null && typeof nvP === "object"
            ? targetsEqual(prevView[i], nvP)
            : isEqual(prevView[i], nvP))
        ) {
          bumpDeep(t);
          bumped = true;
        }
        if (nodes !== null) {
          const node = nodes[i];
          if (node !== undefined) {
            nodesHit++;
            notifyKeyDiff(node, i, old, incoming, false);
          }
        }
      }
    }
    // Indices past the new length (and `length` itself) still have nodes.
    if (nodes !== null && nodesHit < t.nc) {
      for (const key of Reflect.ownKeys(nodes)) {
        const idx = typeof key === "string" ? +key : NaN;
        if (!(idx >= 0 && idx < incoming.length))
          notifyKeyDiff(nodes[key], key, old, incoming, false);
      }
    }
    notifyFoldTail(t, old, incoming);
    return;
  }
  const nodes = t.n;
  let nodesHit = 0;
  for (const k in incoming) {
    const nv = incoming[k];
    let ov = old[k];
    // A key under a guess is judged against the guess too (D-34): a truth
    // equal to the value the guess covered still answers it.
    if (optHooks && ov === nv && prevView !== old) ov = prevView[k];
    const isObj = nv !== null && typeof nv === "object";
    if (
      ov === nv &&
      (!isObj || !isOwned(nv)) &&
      (nodes === null || nodes[k] === undefined || !hasAccessorFlag(nodes[k]))
    ) {
      if (nodes?.[k] !== undefined) nodesHit++;
      continue;
    }
    if (isObj && !shallow) descend(unwrapValue(prevView[k]), nv, keyFn, proj, t, k);
    if (deepUsed && !bumped && !(isObj ? targetsEqual(ov, nv) : isEqual(ov, nv))) {
      bumpDeep(t);
      bumped = true;
    }
    if (nodes !== null) {
      const node = nodes[k];
      if (node !== undefined) {
        nodesHit++;
        notifyKeyValue(node, k, ov, nv, old, incoming);
      }
    }
  }
  const syms = Object.getOwnPropertySymbols(incoming);
  for (let i = 0; i < syms.length; i++) {
    const k = syms[i];
    if (k === $OWNER) continue;
    const nv = incoming[k];
    if (!shallow && nv !== null && typeof nv === "object")
      descend(unwrapValue(old[k]), nv, keyFn, proj, t, k);
    if (nodes !== null) {
      const node = nodes[k];
      if (node !== undefined) {
        nodesHit++;
        notifyKeyValue(node, k, old[k], nv, old, incoming);
      }
    }
  }
  // Keys the incoming object no longer has still have nodes.
  if (nodes !== null && nodesHit < t.nc) {
    for (const key of Reflect.ownKeys(nodes)) {
      // A removed `__proto__` reads absent, not the prototype (core R30,
      // #4672).
      if (!hasOwnP.call(incoming, key))
        notifyKeyDiff(
          nodes[key],
          key,
          old,
          key === "__proto__" ? Object.create(null) : incoming,
          false
        );
    }
  }
  notifyFoldTail(t, old, incoming);
}
const hasOwnP = Object.prototype.hasOwnProperty;
/** Descend into a changed child pair — only where something is proxied
 * below (§6d), never into a raw-marked leaf, never across array/object or a
 * key mismatch (that is a replacement, handled by the parent's slot).
 *
 * A raw in both frames (the same objects swapped, reordered or shifted,
 * #4001) must not leave two slots on one row. A raw already a live row keeps
 * it at the slot it moved to; a row's `pk` follows the slot it sits at (a
 * move or a pairing writes it), so a row whose raw is at that slot of the
 * incoming frame is spoken for and not paired again here; a raw still
 * resolving to a row that moved on drops the lookup (it wraps fresh, or is
 * paired here). One lookup of `nv` on the common path, no allocation. */
function descend(pv, nv, keyFn, proj, t, k) {
  // `nv` is an object at every call site.
  if (pv === null || typeof pv !== "object") return;
  const fam = t._fam;
  nv = unwrapValue(nv);
  let ct = lookupTarget(pv, fam);
  const nt = lookupTarget(nv, fam);
  if (ct !== undefined && ct.pk !== k) {
    if (ct.pk != k && t.v[ct.pk] === ct.v) ct = undefined;
    else if (ct.u === t) ct.pk = k;
  }
  if (nt !== undefined && nt !== ct) {
    if (nt.v === nv) {
      if (nt.u === t) nt.pk = k;
      return;
    }
    (fam?.map ?? storeLookup).delete(nv);
  }
  if (ct === undefined) return; // nothing proxied below this pair
  if (!isWrappable(nv)) return;
  if (rawValuesUsed && isRawValue(nv)) return;
  if (Array.isArray(pv) !== Array.isArray(nv)) return;
  // An array slot was paired by key already (or both rows are keyless).
  if (typeof k !== "number" && keyFn) {
    const pk = keyFn(pv);
    const nk = keyFn(nv);
    if (pk !== undefined && nk !== undefined && !sameKey(pk, nk)) return;
  }
  if (!proj && keyFn !== null && !ct.d) return;
  applyAdopt(ct, nv, keyFn, proj);
}

/**
 * Store — projections (plan sec. 31, S3; INTERNALS §7/§7b).
 *
 * A projection is a computed store: the derive runs inside a computed — the
 * **firewall** — whose pass writes the store through a draft; its output is
 * adopted through the reconcile channel (replace-mode root: entity changes
 * merge in place, the root proxy is stable for life). Children wrap into
 * the projection's own FAMILY (writes land here, never in a source family).
 *
 * The firewall exists so writes from inside a computation are safe (the
 * derive owns its leaves); the price is the ordering obligation: before any
 * value is served through the projection, the derive is brought up to date
 * — **without** the reader subscribing to it (that is the propagation
 * barrier). The store's traps do that pull (`pullFamily`, store.ts): core's
 * `read()` of the firewall with no link — freshness, the status gate
 * (uninitialized / pending / errored) and the reader's height, by the
 * rules a memo's read follows. No per-leaf back-pointer, nothing in core.
 */
/**
 * Wrap a store proxy as a projection DRAFT: every operation carries the write
 * override (the derive is the author — its ops must not hit the status gate,
 * even in a continuation after an `await`/`yield` where the sync write scope
 * has closed).
 *
 * FAKE TARGET, not the store proxy itself (#3060): after a proxy trap
 * returns, the engine runs spec invariant validation against the proxy's
 * TARGET — [[OwnPropertyKeys]] after ownKeys, [[GetOwnProperty]] after
 * set/getOwnPropertyDescriptor/defineProperty. With the store proxy as
 * target those checks re-enter the store's traps OUTSIDE the override
 * bracket, so `Object.keys(state)` in a derive continuation fired the gate.
 */
function wrapDraft(inner, isActive, aroundWrite, shallow, afterWrite, asking) {
  const mutate = op => {
    if (!isActive()) return true;
    setWriteOverride(true);
    // A continuation's write is the flight's answer: it asks the flight's
    // question (A18 provenance — a landing beneath a newer guess is held,
    // not a correction; lanes.ts `laneWrite`).
    const prevQ = question;
    if (asking !== undefined) setQuestion(asking());
    try {
      aroundWrite ? aroundWrite(op) : op();
    } finally {
      setWriteOverride(false);
      setQuestion(prevQ);
    }
    if (afterWrite) afterWrite();
    return true;
  };
  const read = op => {
    setWriteOverride(true);
    try {
      return op();
    } finally {
      setWriteOverride(false);
    }
  };
  const traps = {
    get(_, prop) {
      const value = read(() => inner[prop]);
      return !shallow && typeof value === "object" && value !== null && prop !== $TARGET
        ? wrapDraft(value, isActive, aroundWrite, false, afterWrite, asking)
        : value;
    },
    has: (_, prop) => read(() => prop in inner),
    set: (_, prop, value) =>
      mutate(() => {
        inner[prop] = value;
      }),
    deleteProperty: (_, prop) =>
      mutate(() => {
        delete inner[prop];
      }),
    ownKeys: () => read(() => Reflect.ownKeys(inner)),
    getPrototypeOf: () => protoOf(inner),
    getOwnPropertyDescriptor(_, prop) {
      const d = read(() => Reflect.getOwnPropertyDescriptor(inner, prop));
      if (d) d.configurable = true;
      return d;
    },
    defineProperty: (_, prop, desc) =>
      mutate(() => {
        Reflect.defineProperty(inner, prop, desc);
      })
  };
  const px = new Proxy(Array.isArray(inner) ? [] : {}, traps);
  // The wrapper stands for its store proxy wherever the derive's result
  // carries it (`wrap`, `unwrapValue`, reconcile's slot matching).
  const t = inner[$TARGET];
  if (t !== undefined) setLookup(px, t);
  return px;
}
function cloneState(v, shallow) {
  return shallow ? (Array.isArray(v) ? v.slice() : { ...v }) : JSON.parse(JSON.stringify(v));
}
/** The derive's pass went pending (a new question in flight): every reader
 * of a leaf learns it as a reader of the derive would (A9: any leaf reports
 * the firewall's refetch) — core's own rule for a dependency going pending
 * (`notifyStatus`): a verdict or re-derive reader re-runs, any other is
 * pending derivatively on the derive (its readers hold; it re-runs at the
 * landing). The leaves do not subscribe to the derive, so the family does
 * the walk. */
function wakeFamily(fam) {
  const fw = fam.node;
  // Pending on the derive itself, whatever source took it there (an upstream
  // memo's flight reaches it as a NotReady of that memo, #4271): the leaves
  // are not that source's dependents, so only the family's settle — which
  // retires the derive (`settleFamily`) — reaches the readers parked here. A
  // reader the landing leaves unchanged does not re-run to clear it (#4305).
  const error = new NotReadyError(fw);
  fam._woke = true;
  // A leaf the optimistic engine keeps off the wake (`ask`).
  const skip = optHooks?.ask(fam);
  forEachFamilyNode(fam, n => {
    if (n._subs === null || skip?.(n)) return;
    // The leaf carries the derive's pending as a propagated copy, as a memo
    // between them would: a dependent recovering one branch stays pending
    // through a leaf it still reads (core `retryReaches`), and the landing's
    // walk retires the copy first (`settleFamily` → `settlePendingSource`).
    addPendingSource(n, fw);
    // Its readers: core's propagation from the leaf — a verdict or re-derive
    // reader re-runs, any other is pending derivatively on the derive and
    // queued for the frame (held with it: the frame waits on the flight
    // through them, `blocked`).
    propagateStatus(n, STATUS_PENDING, error);
  });
}
/** The derive's flight rejected: every live reader of the family
 * re-derives (its pull throws the derive's error) — but a reader that
 * already shows this very `error` has had its answer: re-run, its error arm
 * would see the one error twice (D-41: the arm runs once, at the reveal; as
 * core `propagateStatus` re-notifies no reader of an error it carries). A
 * re-run of the derive meets an upstream error again, the same object, when
 * a reader's pull retries it or a held error's commit dirties it (#4457). */
function errorFamily(fam, error) {
  forEachFamilyNode(fam, n => {
    if (n._subs === null) return;
    forEachDependent(n, sub => {
      (sub._statusFlags & STATUS_ERROR && sub._x._error === error) || enqueueSub(sub);
    });
  });
  schedule();
}
/** The flight landed: the readers the wake left pending on the derive
 * settle — core's walk, from each leaf (the leaves are not the derive's
 * dependents, so its own settle walk does not reach them). */
function settleFamily(fam) {
  if (!fam._woke) return;
  fam._woke = false;
  const fw = fam.node;
  forEachFamilyNode(fam, n => {
    if (n._subs !== null) settlePendingSource(n, fw);
  });
  // The landing is the derive's change for a reader parked on the derive
  // itself, whatever status the landing pass began from (an errored derive
  // re-asked: a reader that met the error has no leaf to learn it from).
  insertSubs(fw);
}
/** A source's status reached the derive by propagation (core
 * `propagateStatus`), its pass not re-run: the family learns it as from the
 * pass's own throw. */
function familyStatus(status, error) {
  if (status & STATUS_PENDING) wakeFamily(this);
  // No status: the source's settle walk retired the derive's pending, its
  // pass not re-run (core `settlePendingSource`) — the readers waiting on the
  // derive settle with it (F-9). A re-run would ask again: an async derive's
  // new flight is a second wait, and would run ahead of a staged rejection's
  // publish, which reaches the derive at the commit (F-12, D-41).
  else if (!status) settleFamily(this);
  // The error rides along: a reader the derive's own pass already handed it
  // (a sync derive re-run in the same flush) is not handed it again.
  else if (status & STATUS_ERROR) errorFamily(this, error);
}
function forEachFamilyNode(fam, fn) {
  for (const t of fam.live) {
    const nodes = t.n;
    if (nodes !== null) for (const key of Reflect.ownKeys(nodes)) fn(nodes[key]);
    const has = t.h;
    if (has !== null) for (const key of Reflect.ownKeys(has)) fn(has[key]);
    if (t.k !== null) fn(t.k);
    if (t.dk !== null) fn(t.dk);
  }
}
/** The derive's pass: run `fn` against a draft of the store; commit its
 * result (sync, or each async landing) through the reconcile channel.
 * Draft validity is per run (R37): live from here until the NEXT run starts
 * — a sync derive that subscribes to an external source and pushes into
 * the draft from the callback (#3585) is the model use. */
function runProjectionComputed(wrappedStore, fn, key, wrapCommit, aroundDraftWrite) {
  const owner = getOwner();
  const target = wrappedStore[$TARGET];
  const fam = target._fam;
  const run = (fam.run = (fam.run || 0) + 1);
  // A verdict reader's pass that is the hold's work reads plainly, as a
  // pending node's is (core `recompute`, #3884, D-25), so the landing it
  // computes is the one a reader held on it shows, not the screen a frame
  // reader is served (#4491; D-30, F-2): its pending is on its leaves'
  // readers (`wakeFamily`) with no landing to settle them yet — the settle
  // walk clears the derive's own pending before the landing re-runs it — or
  // a reader of the leaves reads a flight it was served the committed value
  // of (`fam.w`, store.ts `pullFamily`; this pass consumes the mark). For
  // this pass only: its reads set the posture again at its end, and a window
  // in it re-stamps it (`markVerdictReader`). Only the verdict engine makes
  // a verdict reader (`observeFlightHook`).
  // (Out of the hold's verdict lane too, where its seat put the pass — never
  // out of a guess's lane: a derivation of a guess is that lane's, F-6.)
  if (observeFlightHook && (fam._woke || fam.w)) {
    fam.w = 0;
    owner._config &= ~CONFIG_VERDICT;
    if (passLane?._parent._verdict === passLane) setPassLane(null);
  }
  let result;
  // Open loading window (seedLoadingValue): the observable store IS commit #0
  // for the whole first flight — the derive works a detached shadow of the
  // seed so draft writes cannot tear through to readers (#2988). Every
  // commit point reconciles the shadow through the normal commit path.
  const shadow = owner._loading ? cloneState(target.v, target.s) : null;
  // The question a write of this run answers: inside a slice, the slice's
  // (the sync pass); in a continuation — the flight still registered — the
  // flight's (`handleAsync` stamped it with the action that asked).
  const asking = () =>
    question || (owner._x !== null && owner._x._inFlight !== null ? owner._x._q : 0);
  const draft = wrapDraft(
    wrappedStore,
    () => fam.run === run && !isDisposed(owner),
    aroundDraftWrite,
    target.s,
    // A write after the run returned (a continuation, a callback) arms the
    // drain itself when no landing will.
    () => {
      if (!(owner._statusFlags & STATUS_PENDING) && !owner._loading) schedule();
    },
    asking
  );
  // The creation run commits directly (a memo's first value is its
  // `_value`); every later run — a re-derive in a flush, an async landing
  // — stages and commits with the flush like a memo's recompute.
  const first = run === 1;
  const pass = () =>
    storeSetter(
      draft,
      s => {
        result = fn(shadow ?? s);
        const commit = (v, wake = 1) => {
          // Shadow run: commit a detached snapshot, never the shadow itself
          // (adoption takes the value by identity — handing it the live
          // shadow would fuse the draft to the observable store).
          if (shadow && (v === undefined || v === shadow)) v = cloneState(shadow, target.s);
          if (v !== s && v !== undefined) {
            const write = () => {
              const prevQ = question;
              setQuestion(asking());
              try {
                storeSetter(wrappedStore, st => reconcileState(v, st, key, true), false);
              } finally {
                setQuestion(prevQ);
              }
            };
            wrapCommit ? wrapCommit(write, v) : write();
          }
          // A landing (not the sync pass's own commit): the readers held
          // pending on the derive settle — unless core keeps the derive
          // pending: lane work this pass served the committed value of a
          // flight still in the air (core.ts `recompute`, #4862). The lane
          // breaks out of the flight, the family's readers do not (F-4,
          // #4927); they settle at the flight's landing.
          if (
            (!first || !(owner._statusFlags & STATUS_UNINITIALIZED)) &&
            !(
              laneServedHook &&
              owner._statusFlags & STATUS_PENDING &&
              laneServedHook(owner, null, true)
            )
          )
            settleFamily(fam);
          // A commit is the derive's change for the readers that observed its
          // flight (linked by `pullFamily`; a settled pull does not link), as
          // a memo's landing is for its readers: one that caught the NotReady
          // (a fallback value, not pending) re-runs too (#4070). A pass of a
          // settled derive has none (`wake` 0): a link left is a stale one a
          // held frame keeps (A30), from a reader that re-ran since. Woken by
          // every pass, a reader feeding a key the pass peeks at looped (#4342).
          if (wake) insertSubs(owner);
        };
        const land = (value, uninitialized, fail) =>
          landBySetter(owner, value, commit, uninitialized, fail);
        const sync = handleAsync(owner, result, land, (error, pending) => {
          // The flight rejected: the leaves' readers learn it from here —
          // they do not subscribe to the derive. Pending again (a NotReady
          // rejection): the wake, as for the pass. Errored: every live
          // reader re-derives and meets the error at its pull (memo parity,
          // #2897) — and the flush that runs them advances the clock, so a
          // later tracked re-read may retry (core `read`). A held derive's
          // error is staged (D-41, #4489): a reader in the hold derives from
          // the staging and stages its own error with the frame; one served
          // the screen (an untracked read included) reads the committed
          // leaves until the frame reveals.
          if (first && owner._statusFlags & STATUS_UNINITIALIZED) return;
          if (pending) wakeFamily(fam);
          else errorFamily(fam, error);
        });
        // (The status is the one before this pass: `recompute` sets it after.)
        if (!owner._loading) commit(sync, owner._statusFlags & (STATUS_PENDING | STATUS_ERROR));
      },
      false
    );
  try {
    first ? runDirect(pass) : pass();
  } catch (e) {
    // A flight went up (NotReady out of the pass — the derive's own, with
    // the derive as its source): the leaves' readers learn it from here —
    // they do not subscribe to the derive.
    // A synchronous error the same way (#3956: a rerun that finds the
    // entry failed throws): every live reader re-derives and meets it.
    if (!first) e instanceof NotReadyError ? wakeFamily(fam) : errorFamily(fam, e);
    throw e;
  }
  // A pass that returned with a flight up (an iterator's sync first yield):
  // the wake, as for the throw. Not a sync pass that superseded one — the
  // status it reads is the dead flight's, cleared when this pass settles
  // (#3181: a reader the settle walk had just released, re-parked for good).
  if (
    owner._statusFlags & STATUS_PENDING &&
    owner._x._inFlight !== null &&
    !owner._loading &&
    !first
  )
    wakeFamily(fam);
  return owner;
}
/** A derived store's family and root (`opt`: an optimistic store's, its
 * derive `fn` optional — the plain form has none). */
function createProjectionInternal(fn, seed, options, opt) {
  // Seedless (`fn` takes no draft): the root starts as an empty record. It is
  // a draft like any seed, never observable before the first landing (A25).
  seed ||= {};
  const keyOption = options?.key === undefined ? "id" : options.key;
  const fam = {
    map: new WeakMap(),
    node: null,
    live: new Set(),
    st: familyStatus,
    _shallow: !!options?.shallow,
    opt,
    // The row key, for a child paired after the adoption (store.ts
    // `adoptChild`, #3712) as the adoption pairs them.
    key:
      typeof keyOption === "function"
        ? keyOption
        : keyOption === null
          ? null
          : row => (isWrappable(row) ? row[keyOption] : undefined)
  };
  const store = wrap(seed, null, null, fam);
  if (fam._shallow) {
    store[$TARGET].s = true;
    markRawIngest(seed);
  }
  let nodeOptions;
  if (options?.seedLoadingValue) nodeOptions = { loadingValue: undefined };
  if (attrHooks !== null)
    // As createStore: the owner in scope stamps the projection's property
    // nodes, so attribution places them under it (#4035).
    storeOwners.set(store[$TARGET], getOwner());
  if (options?.name || options?.loc) {
    nodeOptions = { ...nodeOptions, name: options.name, loc: options.loc };
    nameStore(store, options.name, options.loc);
  }
  if (fn) {
    const node = computed(() => {
      if (!fam.node) fam.node = getOwner();
      runProjectionComputed(store, fn, keyOption);
    }, nodeOptions);
    node._config &= ~CONFIG_AUTO_DISPOSE;
    fam.node = node;
    // The derive's leaves are not its dependents: a mark's repoll
    // (affects-marks.ts) and a source's status (core `propagateStatus`,
    // `familyStatus`) reach their readers through this.
    node._fam = fam;
  }
  return store;
}
function createProjection(fn, seed, options) {
  return createProjectionInternal(fn, seed, options);
}
/** `createStore(fn, seed, options)`: a projection with a user setter. */
function createStoreDerived(fn, seed, options) {
  const store = createProjectionInternal(fn, seed, options);
  return [store, f => storeSetter(store, f)];
}

/**
 * Store — optimistic stores on lanes (plan sec. 31, S4; INTERNALS §3/§7, RUL-3;
 * S2: a guess is a patch).
 *
 * No store-side layer, no backup snapshots, no retaining ledger: a user
 * write to an optimistic store is one GUESS per target — the user's draft,
 * carried by the target's guess carrier (`kg`, a `$PATCH` slot node like a
 * hold's), with the keys the draft changed recorded on it. Lanes do the rest
 * on that one node (`optimisticWrite`, the same write `createOptimistic`'s
 * setter makes): the guess shows under the action's hold, a landing beneath
 * it stages as truth and confirms it when equal (A18/A24), the action's
 * settle dissolves it (a revert to the frame it covered, or the landed
 * truth). The committed backing is never touched by a guess; a guess folds
 * nothing (its truth lands through its own batch).
 *
 * Reads find the carrier by key (store.ts `frameOf`: a key the guess
 * changed reads through it, tracked, by core's lane rules — what the lane
 * shows; presence, descriptors and enumeration are the patch frame's). A
 * chained target's committed frame is the inner store's proxy: an unguessed
 * key reads through to the inner store's current frame by construction.
 *
 * Landings (Q-D, plan sec. 31.7): the derive's output is reconciled against the
 * optimistic VIEW — a row the guess added and the server returns with the
 * same key in the same position keeps its proxy (the row target adopts the
 * server object) — and reaches the carrier as a truth frame (store.ts
 * `notifyWrites` → `setSignal` → lanes `laneWrite`), judged over the guess's
 * own keys (`echoes`): confirmed beneath, re-based over an older truth
 * (`_laneRebase`), folded (D-34), or a correction.
 */
const hasOwn = Object.prototype.hasOwnProperty;
/** S2 — a target's guess carriers, one per lane (per action): `$PATCH` slot
 * nodes whose lane value is the user's draft, the keys it changed recorded
 * (`wk`, `del`; a keyed list's rows as written and removed keys with it).
 * Oldest first; a carrier that is no lane's is dropped (`sweepOverlaid`).
 * Off the target (a plain store carries nothing for the optimistic engine). */
const carriers = new WeakMap();
const kgOf = t => carriers.get(t) ?? null;
const INDEX = /^(0|[1-9]\d*)$/;
/** The newest live guess carrier for `key`, if any: a lane's guess, or one a
 * correction re-homed into its hold (HELD) — the screen keeps what it showed
 * until that hold lands (A18 (c)), a member reads the truth beneath;
 * `read()` of the carrier decides. Its record (`wk`, `del`) has the key —
 * or, for a read (`reads`), the key is an index or the length of a keyed
 * list's arrangement guess (`g`): a list reader enters the lane through
 * them, as it did through the guessed container. An arrangement guess does
 * not claim a key it only carried over from another live guess's carrier
 * (`borrowed`, #4784): the read falls through to that carrier — or to the
 * truth once it dissolves — instead of serving this frame's stale copy of
 * the expired guess. A row carried over verbatim in a keyed list is not
 * borrowed (`carriedVerbatim`, #4724): the row target decides its contents.
 * A landing asks without `reads`: a key the guess only carried over is the
 * truth's, and its leaf tells its readers as ever (store.ts `leafSet`). */
function guessFor(t, key, shape = false, reads = true) {
  const kg = kgOf(t);
  if (kg !== null)
    for (let i = kg.length - 1; i >= 0; i--) {
      const g = kg[i];
      if (
        g._config & (CONFIG_OVERRIDE | CONFIG_HELD) &&
        (g.wk?.has(key) ||
          g.del?.has(key) ||
          (reads &&
            g.g !== undefined &&
            !g.borrowed?.has(key) &&
            (key === "length" || (typeof key === "string" && INDEX.test(key)))))
      )
        return g;
    }
  return null;
}
/** The newest live guess carrier (the frame a structural read sees). */
function newestGuess(t) {
  const kg = kgOf(t);
  if (kg !== null)
    for (let i = kg.length - 1; i >= 0; i--)
      if (kg[i]._config & (CONFIG_OVERRIDE | CONFIG_HELD)) return kg[i];
  return null;
}
/** The hold's carrier of a truth staged beneath the screen: the container
 * node while a transaction holds its staging (a landing adopted under an
 * action), else null. A guess made over it carries that truth beneath. */
function heldCarrier(t) {
  const k = t.k;
  return k !== null && k._config & CONFIG_HELD && k._pendingValue !== NOT_PENDING ? k : null;
}
/** A guessed key's value from the carrier's frame. */
function carrierValue(frame, key) {
  return hasOwn.call(frame, key) || key !== "__proto__" ? frame[key] : undefined;
}
/** An optimistic setter's targets whose staging was set aside (`draft`),
 * restored when the setter's writes become guesses (`writes`). */
const optStaged = new Map();
/** The view the setter composes on (`writer`): the committed frame — a
 * chained backing resolved through the inner store's — with the lane's
 * value and the tick's own guess over it; `plain`: the committed frame
 * alone (the comparator's world). */
function writerBacking(t, plain = false) {
  const base = committed(t);
  const inner = base[$TARGET];
  const src = inner === undefined ? base : writerBacking(inner, plain);
  return plain ? src : guessView(t, src, true);
}
/** `src` with the target's guess over it: the carrier's unflushed guess or
 * lane value when it carries one, else `src` itself. */
function guessView(t, src, writer = false) {
  const kg = kgOf(t);
  if (kg !== null)
    for (let i = kg.length - 1; i >= 0; i--) {
      const g = kg[i];
      if (writer) {
        // The writer composes on the screen with the tick's own guesses: the
        // unflushed guess, the lane's value, a corrected guess's screen until
        // its hold lands (D-34, D-37 — never the truth beneath it).
        const v = writerFrame(g);
        if (v !== NOT_PENDING) return ownedKeys(t, g, kg, v);
      } else if (g._config & (CONFIG_OVERRIDE | CONFIG_HELD)) {
        // A snapshot, `deep()`, a reconcile's key matching: the carrier as
        // core reads it for this reader (the lane's world, the screen, the
        // truth beneath — the same frame the per-key reads compose).
        const v = untrack(() => read(g));
        if (v !== NOT_PENDING) return v;
      }
    }
  return src;
}
/** A carrier's frame as the writer sees it: the unflushed guess or the
 * lane's value, a corrected guess's screen until its hold lands (D-34). */
function writerFrame(g) {
  if (g._config & CONFIG_OVERRIDE || pendingGuessOf(g) !== NOT_PENDING) {
    const v = guessedValueOf(g);
    if (v !== NOT_PENDING) return v;
  }
  return g._config & CONFIG_HELD ? g._value : NOT_PENDING;
}
/** The newest carrier's frame with each key another carrier owns taken from
 * that carrier, as a read of the key finds it (`guessFor`): the newest frame
 * only copied those keys when it was written, and an older carrier a later
 * guess on the same key went to has moved on since (#4988). A list's frame
 * is its arrangement, taken whole. */
function ownedKeys(t, g, kg, v) {
  let out = v;
  if (!Array.isArray(v))
    for (const o of kg)
      if (o !== g && o.wk !== null)
        for (const key of o.wk)
          if (guessFor(t, key, false, false) === o) {
            const f = writerFrame(o);
            if (f !== NOT_PENDING && f[key] !== out[key])
              (out === v ? (out = { ...v }) : out)[key] = f[key];
          }
  return out;
}
/** The user's draft on an optimistic family: a clone of the view the user
 * saw (#3665). A staging already on the target (a landing adopted this
 * flush) is set aside for the setter's duration: the guess goes over it, it
 * stays the truth beneath. */
function optimisticDraft(t) {
  if (t.pb !== null) optStaged.set(t, t.pb);
  return cloneRaw(writerBacking(t), t);
}
/** The target's guess carrier, made at the first guess: its committed value
 * the frame the guess is shown over. Its comparator judges a truth against
 * the guess over the keys the guess changed (`echoes`: the server's echo of a
 * guessed row is a new object with the same contents, #3898). */
function guessCarrier(t, keys) {
  let kg = kgOf(t);
  if (kg === null) carriers.set(t, (kg = []));
  // The carrier a guess goes on: one with a guess still unflushed (this
  // flush's: one lane), or a live one whose record shares a key (two
  // suggestions for one slot entangle their frames, A34 (1) — as the one
  // leaf node did); else a new one — another action's guess of other keys
  // is its own lane's. A corrected guess whose screen stays until its hold
  // lands (D-34) counts for its keys: the setter composed on that screen,
  // and the guess over it ends with that hold (#4869) — a new carrier over
  // the committed frame would drop a write back to it as no change.
  for (let i = kg.length - 1; i >= 0; i--) {
    const g = kg[i];
    const live = g._config & (CONFIG_OVERRIDE | CONFIG_GUESS);
    if (!live && pendingGuessOf(g) === NOT_PENDING && !(g._config & CONFIG_HELD && keys)) continue;
    if (pendingGuessOf(g) !== NOT_PENDING || (live && keys === undefined))
      return entangle(t, g, keys);
    if (g.wk !== null) for (const key of keys) if (g.wk.has(key) || g.del?.has(key)) return g;
  }
  const g = slotSignal(committed(t), patchEquals, t, $PATCH, false);
  nameCarrier(g, t);
  // Written over a held truth (a landing the target's hold keeps beneath
  // the screen): that truth is the one beneath the guess, judged at the
  // body's end (A18, #3330) — as it was the leaf's staging.
  const p = heldCarrier(t);
  if (p !== null) {
    g._pendingValue = p._pendingValue;
    holdNode(g, txOf(p));
  }
  g.guess = true;
  g.wk = null;
  g.del = null;
  g.removed = null;
  g.borrowed = null;
  g.g = undefined;
  kg.push(g);
  return g;
}
/** This flush's carrier writes a key another action's live guess holds: two
 * suggestions for one slot, and their frames entangle (A34 (1)) — as the one
 * leaf node's lanes merged at the seam (`applyGuesses`): an unapplied
 * carrier is held on that guess's frame, so its lane is born under it; an
 * applied one's frame merges now. */
function entangle(t, g, keys) {
  if (keys !== undefined)
    for (const o of kgOf(t))
      if (o !== g && o._config & CONFIG_OVERRIDE && o.wk !== null)
        for (const key of keys)
          if (o.wk.has(key) || o.del?.has(key)) {
            const holder = resolveTx(txOf(o)._parent);
            if (g._config & CONFIG_OVERRIDE) merge$1(resolveTx(txOf(g)._parent), holder);
            else if (!(g._config & CONFIG_HELD)) holdNode(g, holder);
            break;
          }
  return g;
}
/** The derive's open question is a re-ask of the same one (D-34 ¶3): its
 * own `refresh()`, or every source it waits on re-asked (`refresh(list)`
 * ahead of a sync derive over `list`, #4724) — an input re-asked is not an
 * input changed. A guess written over it stands in for the flight (A17). */
function reasked(d) {
  if (d._flags & REACTIVE_REASK || d._x?._reask) return true;
  const ps = d._x?._pendingSources;
  if (!ps?.size) return false;
  for (const s of ps) if (s === d || !(s._flags & REACTIVE_REASK || s._x?._reask)) return false;
  return true;
}
function patchEquals(a, b) {
  if (a === b) return true;
  if (a === null || b === null || typeof a !== "object" || typeof b !== "object") return false;
  // A chained backing (a proxy) is judged as the inner store's committed
  // frame: the proxy would serve the inner's staging, and a guess equal to
  // that staging is still a guess over the committed world.
  const at = a[$TARGET];
  if (at !== undefined) a = writerBacking(at, true);
  const bt = b[$TARGET];
  if (bt !== undefined) b = writerBacking(bt, true);
  const fam = this._host._fam;
  // A structural guess (a list's, the only kind that records `g`): the
  // arrangement, rows by identity or key, as the container judged it.
  if (this.g !== undefined && (a.length !== b.length || a.some((r, i) => !rowSame(r, b[i], fam))))
    return false;
  // Each key the guess changed, by content: the same object, or the
  // server's echo of a guessed one — a new object with the same contents
  // (#3898). A row the adoption paired by key with a guessed one is not
  // thereby its equal: the leaves judged the content, so does the carrier.
  const wk = this.wk;
  if (wk !== null)
    for (const key of wk) {
      if (key in a !== key in b) return false;
      if (!GlobalQueue._echoes(a[key], b[key])) return false;
    }
  const del = this.del;
  if (del !== null) for (const key of del) if (key in a !== key in b) return false;
  return true;
}
// A keyed arrangement guess carries another guess's row verbatim when the
// setter returned the view's row unchanged: the frame holds the view's row
// object (the `echoes` normalization), and the row target — not the frame
// copy — decides the row's contents, so the arrangement read serves this
// frame's copy instead of falling through to the lender. Falling through
// re-links the reader to the lender's carrier, which loses the
// notification that carries the new arrangement to the DOM (#4724).
// Primitives always borrow: the lender may rewrite the value, and this
// frame's copy would go stale (#4784).
function carriedVerbatim(t, old, pb, key) {
  if (typeof key !== "string" || !INDEX.test(key)) return false;
  const keyFn = t._fam?.key;
  if (keyFn === null || keyFn === undefined) return false;
  const row = unwrapValue(pb[key]);
  return isWrappable(row) && row === old[key];
}
function notifyOptimisticWrites(t, pb) {
  // The view the user saw: the committed frame (a staging adopted eagerly is
  // not it) with the lane's value and the tick's own guess over it.
  const old = writerBacking(t);
  const isArr = Array.isArray(pb);
  const inner = t.ch ? t.v : null;
  const changed = new Set();
  let del = null;
  let structural = false,
    shape = false;
  for (const key of Reflect.ownKeys(pb)) {
    if (key === $OWNER) continue;
    const nv = unwrapValue(pb[key]);
    if (!(key in old)) {
      changed.add(key);
      structural = shape = true;
    } else {
      const previous = old[key];
      // A chained draft holds the inner store's raws: the inner's own frame.
      const ov = unwrapValue(inner !== null && previous ? previous : previous);
      // A row the setter carried over is not a write (#4724, the point of
      // write): the draft's rows are clones of the view's, so a carried row
      // is a new object with the view's contents — the same row, as the
      // server's echo of a guessed one is (`echoes`, #3898).
      if (!isEqual(ov, nv) && !targetsEqual(ov, nv)) {
        if (isArr && echoes(ov, nv)) {
          // The frame carries the row the view showed, not the draft's clone
          // of it: the same object wraps as the same proxy (a list keeps its
          // row identities across a structural guess, deep-chained-view).
          pb[key] = previous;
        } else {
          changed.add(key);
          if (isArr) structural = true;
        }
      }
    }
  }
  for (const key of Reflect.ownKeys(old)) {
    if (key === $OWNER || key in pb) continue;
    (del ??= new Set()).add(key);
    structural = shape = true;
  }
  const staged = optStaged.get(t);
  if (staged !== undefined) optStaged.delete(t);
  if (changed.size === 0 && del === null) return staged ?? null;
  const g = guessCarrier(t, del === null ? changed : new Set([...changed, ...del]));
  // The questions of every action that guessed on this carrier: same-slot
  // guesses share it (D-33), and its `_q` is only the newest's (#4874).
  (g.qs ??= new Set()).add(question);
  // The record accumulates across a lane's writes to the same target.
  if (g.wk === null) g.wk = changed;
  else for (const key of changed) g.wk.add(key);
  if (del !== null) {
    if (g.del === null) g.del = del;
    else for (const key of del) g.del.add(key);
  }
  // The keys this guess's frame only carries over from another live guess's
  // carrier (#4784): the draft was composed on the view, so its frame holds
  // the other guess's values at keys this guess did not itself write. The
  // arrangement read falls through on those keys — to the lender's carrier
  // while it lives, to the truth once it dissolves — instead of serving
  // this frame's copy of the expired guess. A row the setter carried over
  // verbatim is not borrowed (`carriedVerbatim`): the row target decides
  // its contents, and falling through would lose the DOM notification
  // (#4724). The record accumulates across a lane's writes, like `wk`: a
  // key this guess later writes itself is its own, and the `wk`/`del`
  // match above takes precedence regardless. Not a guess that changed the
  // keys or the length: it writes the list itself, the slot the other
  // structural guesses write, so they entangle and its frame holds until
  // they all settle (D-33) — a fall-through there served the truth's
  // length beside this frame's rows.
  const kg = kgOf(t);
  if (kg !== null && kg.length > 1 && !shape && !changed.has("length"))
    for (const o of kg) {
      if (o === g) continue;
      const live =
        o._config & (CONFIG_OVERRIDE | CONFIG_GUESS | CONFIG_HELD) ||
        pendingGuessOf(o) !== NOT_PENDING;
      if (!live) continue;
      for (const key of o.wk ?? [])
        if (!changed.has(key) && !del?.has(key) && !carriedVerbatim(t, old, pb, key))
          (g.borrowed ??= new Set()).add(key);
      for (const key of o.del ?? [])
        if (!changed.has(key) && !del?.has(key) && !carriedVerbatim(t, old, pb, key))
          (g.borrowed ??= new Set()).add(key);
    }
  if (structural && isArr) {
    const rows = pb.map(arrangementRow, t._fam);
    // The keys of the view the setter saw that are not the guess's own
    // rows, which a re-base over another truth takes from that truth
    // (`_laneRebase`): the rows the setter removed, which it never brings
    // back, and the rows it only carried over from the truth beneath (the
    // committed row itself), which the new truth decides. Rows it added, or
    // carried from another action's guess, are the guesses' own: their
    // landings judge them. A set (empty or not) marks a keyed arrangement
    // guess (`fold`).
    let removed = null;
    const keyFn = t._fam.key;
    if (keyFn !== null && keyFn !== undefined) {
      removed = new Set();
      const kept = new Set();
      const base = committed(t);
      const truth = new Set(base);
      for (let i = 0; i < rows.length; i++) {
        const k = isWrappable(rows[i]) ? keyFn(rows[i]) : undefined;
        if (k !== undefined) kept.add(k);
      }
      // The view's rows, then the truth's: a truth row the view lacked was
      // removed by another action's guess, whose landing judges it — a
      // re-base never brings it back beside that guess (F-11, #4656).
      const n = old.length;
      const all = [...old, ...base];
      for (let i = 0; i < all.length; i++) {
        const r = unwrapValue(all[i]);
        const k = isWrappable(r) ? keyFn(r) : undefined;
        if (k !== undefined && (!kept.has(k) || (i < n && truth.has(r)))) removed.add(k);
      }
    }
    g.removed = removed;
    g.g = rows;
    // Written while the derive asks a new question (an input changed, not a
    // `refresh()`): the guess's base answers the old one, so the rows it only
    // carried over are that answer's — the new answer re-bases it like an
    // older truth instead of folding beneath it (`_laneRebase`, F-1, #4525).
    // A refresh's flight already up (`_reask`, the pass consumed the flag)
    // asks nothing new either (D-34 ¶3, #4656).
    const d = t._fam.node;
    if (removed && d?._statusFlags & STATUS_PENDING && !reasked(d)) g.g = null;
  }
  // By value: the draft is the guess as written (a function leaf is a
  // value, not an updater, #3017).
  optimisticWrite(g, () => pb);
  if (t.dk !== null) optimisticWrite(t.dk, {});
  markOverlaid(t);
  return staged ?? null;
}
// A borrowed row must keep following its source through a re-base. Unwrapping
// a foreign proxy here would retain its old backing after that source adopts
// a new answer (#4931). Local rows still use the family's raw identity.
// (`this`: the family, as `rows.map(arrangementRow, fam)`.)
function arrangementRow(row) {
  return row?.[$TARGET]?._fam !== this ? row : unwrapValue(row);
}
/** Two rows of a keyed list's arrangement, by identity — by key when the
 * family has one. */
function rowSame(a, b, fam) {
  if (isEqual(a, b) || targetsEqual(a, b)) return true;
  const keyFn = fam.key;
  if (!keyFn || !isWrappable(a) || !isWrappable(b)) return false;
  // A row the store already serves is its proxy's row: a reader (a list)
  // holds that proxy, so another object at the index is another row, by key
  // or contents alike — a guess's fresh object wraps a new proxy. A landing's
  // object has been adopted by the row it pairs with, so it serves that
  // row's proxy. Only rows no proxy serves yet pair by key, keyless rows by
  // position — reconcile's fallback (#3898).
  const ta = lookupTarget((a = unwrapValue(a)), fam);
  const tb = lookupTarget((b = unwrapValue(b)), fam);
  if (ta || tb) return ta === tb;
  // (Two keyless rows: `undefined` is the same key.)
  return sameKey(keyFn(a), keyFn(b));
}
/** Whether reconcile would write anything into `a` from `b`: wrappable
 * pairs descend, leaves compare as its writes do. A key mismatch is a
 * replacement, which a content difference already says. Plain data — the
 * server's echo of a guessed row is a new object with the same contents. */
function echoes(a, b) {
  if (a === b) return true;
  if (!isWrappable(a) || !isWrappable(b)) return false;
  a = unwrapValue(a);
  b = unwrapValue(b);
  const ak = Object.keys(a);
  if (Array.isArray(a) !== Array.isArray(b) || ak.length !== Object.keys(b).length) return false;
  for (const k of ak) if (!hasOwn.call(b, k) || !echoes(a[k], b[k])) return false;
  return true;
}
// A cycle (a row that references itself) is not plain data: no match, so
// the landing stays a correction.
GlobalQueue._echoes = (a, b) => {
  try {
    return echoes(a, b);
  } catch {
    return false;
  }
};
/** Write `incoming` into the draft `draft` (a store proxy inside a user's
 * setter): arrays match rows by key (position when unkeyed) and recurse into
 * the matched row's draft; objects recurse into a wrappable pair, assign
 * otherwise; keys `incoming` lacks are deleted. */
function reconcileDraft(draft, incoming, keyFn) {
  if (Array.isArray(incoming)) {
    const n = draft.length;
    const prev = new Array(n);
    for (let i = 0; i < n; i++) prev[i] = draft[i];
    let byKey = null;
    if (keyFn !== null) {
      byKey = new Map();
      for (let i = 0; i < n; i++) {
        const raw = unwrapValue(prev[i]);
        if (!isWrappable(raw)) continue;
        const k = keyFn(raw);
        if (k !== undefined && !byKey.has(k)) byKey.set(k, prev[i]);
      }
    }
    for (let i = 0; i < incoming.length; i++) {
      const nv = incoming[i];
      let match;
      if (isWrappable(nv)) {
        if (byKey !== null) {
          const k = keyFn(nv);
          if (k !== undefined) match = byKey.get(k);
        } else if (i < n && isWrappable(unwrapValue(prev[i]))) match = prev[i];
      }
      if (match !== undefined) {
        if (unwrapValue(match) !== nv) reconcileDraft(match, nv, keyFn);
        if (draft[i] !== match) draft[i] = match;
      } else if (unwrapValue(draft[i]) !== nv) draft[i] = nv;
    }
    if (draft.length !== incoming.length) draft.length = incoming.length;
    return;
  }
  for (const k of Object.keys(incoming)) {
    const nv = incoming[k];
    const cur = draft[k];
    const curRaw = unwrapValue(cur);
    if (
      isWrappable(nv) &&
      isWrappable(curRaw) &&
      Array.isArray(nv) === Array.isArray(curRaw) &&
      (keyFn === null || Array.isArray(nv) || sameKey(keyFn(nv), keyFn(curRaw)))
    ) {
      if (curRaw !== nv) reconcileDraft(cur, nv, keyFn);
    } else if (curRaw !== nv) draft[k] = nv;
  }
  for (const k of Object.keys(draft)) if (!(k in incoming)) delete draft[k];
}
/** Core asks (scheduler `laneDirty`): the lane dirtied the target's guess
 * carrier — its readers are the readers of the leaves of the keys the guess
 * changed (they link their leaves, and the carrier once they read through
 * it): dirtied as the lane's work, with the presence nodes and the
 * container (a structural guess). */
/** The nodes the readers of a guess carrier's keys link: the leaves and
 * presence nodes of the keys it changed, and the container. */
function readersOf(n, fn) {
  if (n._key !== $PATCH || !n.guess) return;
  const t = n._host;
  const each = keys => {
    for (const key of keys) {
      const leaf = t.n?.[key];
      if (leaf !== undefined && leaf._subs !== null) fn(leaf);
      const has = t.h?.[key];
      if (has !== undefined && has._subs !== null) fn(has);
    }
  };
  each(n.wk ?? []);
  if (n.del !== null) each(n.del);
  const k = t.k;
  if (k !== null && k._subs !== null) fn(k);
}
GlobalQueue._slotDirty = (n, l) => readersOf(n, node => laneDirty(node, l));
/** Core asks (lanes `rerun`): a guess coming back at a landing re-derives
 * the carrier's readers there — the readers of its keys' leaves. */
GlobalQueue._slotRerun = (n, t) =>
  readersOf(n, node => {
    for (let s = node._subs; s !== null; s = s._nextSub) staleReader(s._sub, t);
  });
/** Core asks (lanes `laneWrite`): the family's derive, re-asking in a lane of
 * another action than the one that guessed this target, writes it — that
 * lane's new question, which the guess (over the old one) does not answer:
 * the carrier is the lane's derivation from here (F-6, #4336; lanes
 * `laneStage` for a node's own pass). */
GlobalQueue._slotTake = (n, back) => {
  const t = n._host;
  if (back) {
    // The guess comes back (lanes `endLanes`): on the target's carrier — the
    // node itself, re-attached if the taker lane's end swept it (as the leaf
    // path re-made its node).
    markOverlaid(t);
    let kg = kgOf(t);
    if (kg === null) carriers.set(t, (kg = []));
    if (kg.indexOf(n) === -1) kg.push(n);
    return n;
  }
  const d = t?._fam?.node?._x?._transaction;
  const g = txOf(n);
  if (!d?._lane || sameAction(d, g)) return false;
  // The re-ask takes the target's guess whole (the carrier and the deep
  // witness): a node other than the written one shows its truth in the
  // lane's world (lanes `take`).
  for (const m of [...(kgOf(t) ?? []), t.dk])
    if (m?._config & CONFIG_GUESS && txOf(m) === g) take(m, d, m !== n);
  return true;
};
/** Q-D (plan sec. 39): an OLDER question's truth landed beneath a guess
 * (`laneWrite` held it — another action's landing is not this guess's
 * answer). The guess's edit is re-based over it: the truth frame with the
 * guess's own keys over it; a keyed list's order from the guess, rows from
 * the truth by key (a guessed row the truth has is the truth's object — the
 * row target adopted it, its proxy survives), rows the guess added — or
 * carried from another action's guess — kept (their landings judge them),
 * truth rows the guess lacks appended unless it removed them. A row the
 * guess only carried over from the truth it was written on is that truth's,
 * not the guess's: the new truth decides it, so after a new question the
 * old list's rows never show beside the new one's (F-1, D-35, #4482;
 * `removed`). Unkeyed lists compose positionally — the guess stands as
 * written (the documented residue). A current truth re-bases, the same way,
 * a guess written while its question was being asked (`g` null, #4525):
 * asked before the guess, it is the older question; its rows are held
 * beneath the guess. True: the lane holds the landing (`laneWrite`). */
GlobalQueue._laneRebase = (el, truth, older) => {
  const t = el._host;
  // Not a store's: the lane judges the write (`supersede`).
  if (t === undefined) return false;
  if (el._key !== $PATCH) {
    // The deep witness's guess: a bump beneath a shown keyed arrangement
    // guess folds that lane (D-34), else the lane judges it.
    if (older) return true;
    const g = kgOf(t)?.find(c => c._config & CONFIG_OVERRIDE && c.removed);
    return g !== undefined && g.g !== null ? fold(g, truth) : false;
  }
  const guess = latestOf(el);
  // A current truth: folded beneath a shown keyed arrangement guess (D-34).
  // Not folded (an object's guess, a lane that has not shown): held beneath
  // the guess for the hold's end, as a leaf's landing was — the screen keeps
  // the guess until the commit (D-34; #4690, the outside re-ask).
  if (!older && el.g !== null) return fold(el, truth);
  // The truth of an action whose own guess is on this carrier, entangled
  // with a later one's (same slot, D-33): the hold's own staging, beneath
  // the guess until the hold commits — none of it shows before (#4874).
  if (older && el.qs?.has(question)) return true;
  // The truth answers another carrier's guess (`by`, its rows): the rows
  // this guess carried over from that guess are judged by it — its own
  // landing — so a placeholder it added never shows beside the row its
  // answer wrote (#4897). Entangled with this guess's hold (an action that
  // read that guess, D-33, #4788), that truth is the hold's own staging, as
  // a same-carrier one above: none of it shows before the hold commits.
  let by;
  if (older)
    for (const o of kgOf(t))
      if (o !== el && o.qs?.has(question)) {
        if (sameAction(txOf(o), txOf(el))) return true;
        by = o.g;
      }
  // An older question's truth, or the #4525 re-base: the guess's edit over it.
  if (!Array.isArray(truth) || !Array.isArray(guess)) return true;
  const keyFn = t._fam?.key;
  if (!keyFn) return;
  const byKey = new Map();
  for (let i = 0; i < truth.length; i++) {
    const r = truth[i];
    const k = isWrappable(r) ? keyFn(r) : undefined;
    if (k !== undefined && !byKey.has(k)) byKey.set(k, r);
  }
  const used = new Set();
  const removed = el.removed ?? new Set();
  const rows = [];
  // From the guess as written, not an earlier re-base's composition: a
  // derive's landing writes key by key, and a truth row one partial write
  // showed must not stay appended once the landing drops it (#4441).
  // The #4525 re-base (`g` null): the lane's rows are the new question's
  // answer now; its work reads that question's hold as the hold's own
  // flush does (core.ts `frameRead`, F-1, #4536).
  if (el.g === null) txOf(el).over = t._fam.node._x._transaction;
  const guessed = (el.g ??= guess.map(arrangementRow, t._fam));
  for (let i = 0; i < guessed.length; i++) {
    const g = guessed[i];
    const k = isWrappable(g) ? keyFn(g) : undefined;
    const tr = k !== undefined ? byKey.get(k) : undefined;
    if (tr !== undefined) {
      // The truth's row under the guessed one's key: the guessed row's
      // target adopts it (the same proxy from here; its leaves tell its
      // readers what changed), as the derive's draft pairs new rows under
      // guessed ones (`pairRows`).
      const gt = isWrappable(g) ? lookupTarget(unwrapValue(g), t._fam) : undefined;
      if (gt !== undefined && lookupTarget(tr, t._fam) === undefined)
        reconcileState(tr, gt.px, keyFn, true);
      rows.push(tr);
      used.add(k);
    } else if (!removed.has(k) && !by?.includes(g)) rows.push(g);
  }
  for (let i = 0; i < truth.length; i++) {
    const r = truth[i];
    const k = isWrappable(r) ? keyFn(r) : undefined;
    if (k !== undefined && !used.has(k) && !removed.has(k)) rows.push(r);
  }
  // Every index of the composition is the guess's from here (a new row at
  // an index, the length): the record covers the frame.
  const wk = el.wk ?? (el.wk = new Set());
  const n = Math.max(rows.length, guess.length, committed(t).length);
  for (let i = 0; i < n; i++) wk.add(String(i));
  wk.add("length");
  const l = txOf(el);
  setLaneValue(el, l, rows);
  // The composition changed what the lane shows (a truth row's content
  // under a guessed key, a row appended): the carrier's readers re-derive
  // as the lane's work (`laneDirty`; the leaf's `laneSet` did).
  if (!sameRows(guess, rows, t._fam)) laneDirty(el, l);
  return true;
};
/** Two row lists show the same through the proxies: each slot the same
 * target (a row the guessed one's target adopted is the same proxy) or
 * the same primitive. */
function sameRows(a, b, fam) {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    const x = unwrapValue(a[i]),
      y = unwrapValue(b[i]);
    if (isWrappable(x)) {
      const xt = lookupTarget(x, fam);
      if (xt === undefined || xt !== lookupTarget(y, fam)) return false;
    } else if (!isEqual(x, y)) return false;
  }
  return true;
}
/** D-34 for a keyed arrangement guess: rows by key, not positions. A
 * current truth that differs folds into the hold beneath it instead of
 * voiding the lane: the guess stays the lane's value and the screen's until
 * the hold commits, where rows match by key, and the graph outside the lane
 * re-derives from the truth now (`foldedRead`). A confirmation is silent
 * (A17): only truth readers are told. True: the lane stages the truth
 * beneath (`laneWrite`). */
function fold(el, truth) {
  const l = txOf(el);
  // A guess the screen never showed is void at the correction (D-34); a
  // keyed arrangement guess's carrier carries its removed-keys set.
  if (!l._shown || !el.removed) return false;
  const guess = latestOf(el);
  if ((!el._equals || !el._equals(guess, truth)) && !GlobalQueue._echoes(guess, truth))
    l.folded = foldedRead;
  else GlobalQueue._confirmed?.(l);
  // A folded lane's `latest()` serves the truth beneath the guess
  // (`foldedRead`): every truth landing there is news to its readers, a
  // confirming one too — they re-derive from it (D-34, F-6).
  if (l.folded) insertSubs(el);
  joinFuture(resolveTx(l._parent ?? l));
  return true;
}
/** A folded lane's guess read by a deriving pass outside the lane, tracked
 * or not (`untrack` is about dependencies, not about which world a pass
 * derives from), or by `latest()` (`c` null): the hold's world — the truth
 * staged beneath the guess, else the value it covered — and the pass joins
 * the hold. Lane passes and the screen keep the guess: held truth is masked
 * from lane passes only (#3164). The store's own derive reconciles its
 * landings against the arrangement the guesses made: it is the author, not
 * a reader. */
function foldedRead(c, el) {
  if (c !== null) {
    if (
      !(el._config & CONFIG_GUESS) ||
      passLane !== null ||
      c._type === EFFECT_RENDER ||
      c._config & CONFIG_CHILDREN_FORBIDDEN ||
      el._host?._fam?.node === c
    )
      return NOT_PENDING;
    const l = txOf(el);
    joinPassAt(c, resolveTx(l._parent ?? l));
  }
  if (el._pendingValue !== NOT_PENDING) return el._pendingValue;
  return covered(el);
}
/** D-34 for a per-key guess (#4329): the derive's draft wrote new objects
 * over a target holding guessed rows (`d.length = 0; d.push(...rows)`, or
 * `d.issues = rows`). The container on the way down to each guessed row
 * reconciles the new object at its slot — in a keyed list, the one with
 * the row's key — exactly as a returned landing would: rows match by key,
 * a guessed row keeps its target, and its leaves are judged against their
 * guesses. With no guess out, rows nothing guessed are left as the draft
 * wrote them. */
function pairRows(t, pb, viewed) {
  const fam = t._fam;
  const keyFn = fam.key;
  const list = Array.isArray(pb);
  // One rule (Q-D, plan sec. 31.7): a row with a target keeps it under the
  // landing's new object at its slot — in a keyed list, the one with its
  // key (an unkeyed list pairs nothing). The target adopts the object; a
  // row another store lent keeps its proxy in the slot.
  let at;
  const pair = (k, px, lent) => {
    if (list && at === undefined) {
      at = new Map();
      pb.forEach((r, i) => {
        const rk = isWrappable(r) && keyFn ? keyFn(r) : undefined;
        if (rk !== undefined && !at.has(rk)) at.set(rk, i);
      });
    }
    // A returned landing leaves a row another store lent alone (its target
    // here is chained over that store's proxy, §7b): the diff against the
    // view keeps it by key, and reconciling the answer into it ties the
    // lender's row to this landing, so two stores that lent each other rows
    // waited on each other's flight forever (#4928).
    if (viewed && px[$TARGET].ch) return;
    const i = list ? at.get(k) : k;
    const nv = i === undefined ? undefined : pb[i];
    // A new object only: not the row's own, nor one another row serves
    // (pairing it would alias the two).
    if (isWrappable(nv) && !lookupTarget(nv, fam)) {
      reconcileState(nv, px, keyFn, true);
      if (lent) pb[i] = px;
    }
  };
  // A guessed row, or its ancestor directly under `t`.
  for (let c of fam._overlaid ?? []) {
    while (c.u !== t) if ((c = c.u) === null) break;
    if (c !== null) pair(list ? keyFn?.(c.v) : c.pk, c.px);
  }
  if (!keyFn || !list) return;
  // While guesses are out, every committed row, not only a guessed one
  // (#4738): a row whose guess settled can be guessed again, and the setter
  // writes the committed row's target.
  if (fam._overlaid?.size)
    for (const ov of committed(t)) {
      const rt = isWrappable(ov) ? lookupTarget(ov, fam) : undefined;
      if (rt !== undefined) pair(keyFn(ov), rt.px);
    }
  // A row another store's guess added as it is (that store's proxy, not a
  // copy): a guess of that store's still out on the row shows on through
  // its own carrier until its action ends (F-11, #4902). A returned landing
  // (`viewed`) is diffed against the view, which already holds the lent row
  // by key: its slot stays this store's row (#4918).
  if (!viewed)
    kgOf(t)?.forEach(c => {
      const f = guessedValueOf(c);
      if (Array.isArray(f))
        for (const r of f) {
          const ft = r?.[$TARGET];
          if (ft?.px === r && ft._fam !== fam) pair(keyFn(r), r, true);
        }
    });
}
/** #4788 (D-33): an action read `key` of `t` (`v`, else the writer's view
 * of it): when a live guess wrote that key, a guess the action writes next
 * derives from it (core lanes.ts `readGuesses`). A row reached through a
 * guessed index, or the length of a guessed arrangement, is navigation, not
 * a value the write is computed from: the row's own keys say what was read
 * of it. Nor is the keyed diff a returned array is written by (`diffing`):
 * the user's own reads are the setter's. */
let diffing = false;
/** #4812: a record a live guess put in place (a replaced child), with the
 * record it replaced: what the setter reads under it derives from that
 * guess, however deep — where it differs from what the replaced record
 * holds (a rewritten row's unchanged `id` is navigation, #4724). */
const placed = new WeakMap();
function noteRead(t, key, v) {
  if (!actionDepth || diffing) return;
  const g = guessFor(t, key, false, false);
  const p = g && g._config & CONFIG_OVERRIDE ? null : placed.get(t.v);
  if (!p && !(g && g._config & CONFIG_OVERRIDE)) return;
  const l = p ? p[0] : txOf(g),
    old = unwrapValue(p ? p[1]?.[key] : t.v[key]),
    r = unwrapValue(v ?? (g ? carrierValue(guessedValueOf(g), key) : t.v[key]));
  if (isWrappable(r)) r !== old && placed.set(r, [l, old]);
  else if (!p || !Object.is(r, old)) readGuesses.push(l, question);
}
/** #4812: presence (`in`) and enumeration in a setter's draft read the key
 * set a live guess changed — a key it added or deleted. */
function noteShape(t, key) {
  const kg = actionDepth && !diffing ? kgOf(t) : null;
  if (kg !== null)
    for (let i = kg.length - 1; i >= 0; i--) {
      const g = kg[i];
      if (
        g._config & CONFIG_OVERRIDE &&
        (key !== undefined
          ? g.del?.has(key) || (g.wk?.has(key) && !(key in t.v))
          : g.del?.size || [...(g.wk ?? [])].some(k => !(k in t.v)))
      )
        readGuesses.push(txOf(g), question);
    }
}
installOptHooks({
  pair: pairRows,
  draft: optimisticDraft,
  writes: notifyOptimisticWrites,
  view: guessView,
  reconcile: (d, v, k) => {
    diffing = true;
    reconcileDraft(d, v, k);
    diffing = false;
  },
  sweep: sweepOverlaid,
  // The carrier arms (store.ts calls them for an optimistic family's target
  // outside a draft and the write override): a key a live guess changed
  // reads through the carrier, tracked — the lane's seat, a verdict
  // window's judgment of the guess (A17/A24) — and the key's leaf stays the
  // reader's subscription point (silent while the guess covers it: the next
  // guess on the key, on a carrier of its own, reaches the reader through
  // it, `readersOf`). Presence and descriptors are the carrier frame's;
  // enumeration reads the newest carrier, tracked.
  read: (t, key) => {
    const g = guessFor(t, key);
    const obs = getObserver();
    if (g === null) {
      // A key an arrangement guess only borrowed, its lender gone, reads the
      // truth (#4784), but that guess still orders the rows read beside it:
      // its end must reach this reader, as when it served the key (#4897).
      if (obs !== null) kgOf(t)?.forEach(c => c.borrowed?.has(key) && link(c, obs));
      return NOT_PENDING;
    }
    const frame = read(g);
    const node = obs !== null ? getNode(t, key) : t.n?.[key];
    if (obs !== null) link(node, obs);
    const v = carrierValue(frame, key);
    if (obs === null) noteRead(t, key, v);
    return serveKeyValue(t, key, v, false, node);
  },
  drafted: (t, key, shape) => (shape ? noteShape(t, key) : noteRead(t, key)),
  has: (t, key) => {
    const g = guessFor(t, key, true);
    if (g === null) return undefined;
    const obs = getObserver();
    if (obs !== null) link(getHasNode(t, key), obs);
    return key in read(g);
  },
  descriptor: (t, key, array) => {
    const g = guessFor(t, key, true);
    if (g === null) return undefined;
    const obs = getObserver();
    if (obs !== null) link(getHasNode(t, key), obs);
    const frame = read(g);
    if (!(key in frame)) return null;
    const value = frame[key];
    const length = key === "length" && Array.isArray(frame);
    return { value, writable: true, enumerable: !length, configurable: !(length && array) };
  },
  frame: t => {
    const g = newestGuess(t);
    return g === null ? NOT_PENDING : read(g);
  },
  // A key a live guess changed: its leaf is silent at a landing — its
  // readers read the carrier, whose judgement of the landing tells them (a
  // correction through the carrier, a confirmation silently, A17).
  covers: (t, key, shape) => guessFor(t, key, shape, false) !== null,
  // A landing reaches each live carrier as the truth it is judged against
  // (`laneWrite`: confirmed beneath, re-based, folded, or a correction) —
  // the frame as landed, not the backing by reference: a later batch writes
  // the backing in place, and a truth staged beneath a guess (a correction
  // held for the hold's end) must stay the answer it was.
  judge: (t, truth) => {
    const kg = kgOf(t);
    if (kg === null) return;
    const frame = Array.isArray(truth) ? truth.slice() : { ...truth };
    for (let i = 0; i < kg.length; i++)
      if (kg[i]._config & (CONFIG_OVERRIDE | CONFIG_HELD)) setSignal(kg[i], frame);
  },
  // A new question (not a `refresh()`) asked over guesses an earlier frame
  // opened, which answer the old one (D-34, #4336). Stamped on `fam._asked`
  // with this flush's `clock`, so it lapses when the flush ends: `clock + 1`
  // from mainline over a live guess — a frame still reading a guess at the
  // seam shows it beside the new input, so the flush holds with the guess's
  // action (`park`); `~clock` from a lane with no guess of its own, or with
  // no live guess left — the lane waits for its answer (F-6), as for a
  // guessed `createOptimistic` (lanes `laneStage`). The guess's own lane
  // asks nothing new and stamps nothing.
  // A `refresh()` re-asks the guesses' own question (D-34 ¶3) — the
  // derive's own, or every source it waits on re-asked (`reasked`: a
  // refreshed memo dirties a sync derive ahead of its own refresh, #4801):
  // a guess
  // stands in for its flight (A17), so a leaf lane work reads is kept off
  // the wake — the lane shows its guesses and the flight holds the lane's
  // parent, not the lane (F-11, #4656).
  ask: fam => {
    if (!fam._overlaid?.size) return;
    if (reasked(fam.node)) return n => n._subs._sub._x?._transaction?._lane;
    // Stamped with the flush (`clock`): it lapses when the flush ends.
    let a = ~clock;
    for (const g of fam._overlaid) {
      const n = laneNodeOf(g);
      if (n) {
        if (passLane === null) a = clock + 1;
        else if (sameAction(txOf(n), passLane)) return;
      }
    }
    if ((fam._asked = a) > 0) joinFuture(null);
  },
  // Over a new question the guesses do not answer (`_asked`): from mainline
  // only a target carrying a guess stands in — a read of one with none is an
  // ordinary pending read (F-2), so a keyed store's other keys wait for their
  // answer, not for the guess's action; in a lane with no guess of its own,
  // or with no live guess left, none does (F-6) — whoever asked: a reader
  // that joined another action's lane (it reads that action's guess
  // elsewhere) over mainline's new question waits for its answer, so a
  // `Loading on` region remounted under it shows its fallback (#4929).
  // A reader already observing the flight (linked) keeps observing it:
  // served the guess, a re-run (the guess's own, the seam) would drop its
  // link and release the hold beside a guess over the previous answer
  // (F-1, #4536) — the ordinary read serves its committed frame or keeps it
  // pending. So does a reader in the frame's hold pending on a flight a list
  // guess was written over (`g` null, the #4525 mark; D-34: the guess does
  // not answer it): a render effect is read here, as `pullFamily`'s own read
  // would, since the frame's guard there returns early for an unlinked one.
  // A derivation a pending render effect reads (#4561), through any chain
  // of derivations (`held`), keeps observing a held flight as a stale reader
  // of its hold — linked, re-derived at the landing, so the hold still sees
  // its frame (`blockedBy`) — and is served the guess like lane work (a
  // `<For>` keeps showing it, #4525), its every read this pass (D-34 ¶3:
  // however the reads are grouped); a flight no transaction holds (a
  // `Loading on` fallback) it reads plainly.
  stands: t => {
    const f = t._fam;
    const o = f._overlaid;
    const a = f._asked;
    const d = f.node;
    const c = context;
    if (
      !o?.size ||
      (a === clock + 1 &&
        (passLane === null
          ? !o.has(t)
          : ![...o].some(g => (g = laneNodeOf(g)) && sameAction(txOf(g), passLane)))) ||
      a === ~clock
    )
      return false;
    // Linked by this pass's hold (below): its other reads keep the guess.
    const j = heldC === c && heldAt === clock;
    if (linkedTo(c, d)) return j && heldV > 1;
    if (kgOf(t)?.some(g => g.g === null) && c?._statusFlags & STATUS_PENDING) {
      if (c._type === EFFECT_RENDER) read(d);
      else if (j ? heldV : ((heldC = c), (heldAt = clock), (heldV = held(c))))
        d._x?._transaction ? (link(d, c), staleReader(c, txOf(d)), (heldV = 2)) : read(d);
    }
    // #4786: guesses of one action standing in (A17) for a `refresh()` that
    // another action holds (D-34 ¶3: the same question) still leave the
    // pass derived from that refetch: a derivation reading them is a stale
    // reader of it (A15). Linked, the refetch's hold keeps waiting for it
    // (F-11) and it re-derives at the landing. A render effect reads its
    // memo's frame; a new question is the branches above.
    else if (
      c?._depsTail !== undefined &&
      c._type !== EFFECT_RENDER &&
      d._x?._reask &&
      d._x._transaction &&
      !ownFlight(o, txOf(d))
    )
      (link(d, c), staleReader(c, txOf(d)));
    return true;
  },
  // Mainline hold (`clock + 1`): the flush joins the action of each guess a
  // reader still reads at the seam; a reader that moved off it (a keyed
  // store's other key) holds nothing.
  park: t => {
    for (const fam of overlaidFamilies)
      if (fam._asked === clock + 1)
        for (const g of fam._overlaid) {
          const n = laneNodeOf(g, true);
          if (n) merge$1(txOf(n)._parent, t);
        }
  }
});
/** A derivation in the frame's hold (scheduler.ts `blockedBy`): one a
 * pending render effect reads, through any chain of pending derivations
 * (#4561: memo -> memo -> binding, `mapArray` -> memo -> binding). Each
 * derivation is walked once (`seen`): a diamond graph stays linear. */
function held(c, seen = new Set()) {
  for (let s = c._subs; s; s = s._nextSub) {
    const r = s._sub;
    if (
      r._statusFlags & STATUS_PENDING &&
      (r._type === EFFECT_RENDER || (!r._type && !seen.has(r) && (seen.add(r), held(r, seen))))
    )
      return true;
  }
}
/** The last reader judged (`heldC`) this pass (`heldAt`, the pass's `clock`)
 * and its verdict (`heldV`: 2 once this pass linked it): a reader's key reads
 * come in one run, so each does not re-walk its subscribers. */
let heldC, heldAt, heldV;
/** A guess in `o` (live or held) belongs to the transaction `f` that holds
 * the flight it stands in for. */
function ownFlight(o, f) {
  for (const t of o)
    for (const n of [...(kgOf(t) ?? []), t.dk])
      if (n?._x?._transaction) {
        const l = txOf(n);
        if (resolveTx(l._parent ?? l) === f) return true;
      }
  return false;
}
function sameAction(a, b) {
  return resolveTx(a._parent ?? a) === resolveTx(b._parent ?? b);
}
/** Families with a target carrying a lane value — swept at the flush's
 * commit (`sweepOverlaid`): a target whose carrier is no lane's any more
 * leaves its family's `_overlaid` and drops the carrier. (Not on the read
 * path: a guess is pending between the setter and the seam, and a read in
 * between must not take the target for clean.) */
const overlaidFamilies = new Set();
function markOverlaid(t) {
  const fam = t._fam;
  (fam._overlaid ??= new Set()).add(t);
  overlaidFamilies.add(fam);
}
function sweepOverlaid() {
  if (overlaidFamilies.size === 0) return;
  for (const fam of overlaidFamilies) {
    const set = fam._overlaid;
    for (const t of set)
      if (!laneNodeOf(t)) {
        set.delete(t);
        const live = kgOf(t)?.filter(
          g => g._config & CONFIG_HELD || g._pendingValue !== NOT_PENDING
        );
        if (!live || live.length === 0) carriers.delete(t);
        else carriers.set(t, live);
      } else if (kgOf(t) !== null) {
        // A carrier that is no lane's and no hold's (a corrected guess waits
        // for its hold's landing, A18 (c)) is done.
        const live = kgOf(t).filter(
          g =>
            g._config & (CONFIG_OVERRIDE | CONFIG_GUESS | CONFIG_HELD) ||
            g._pendingValue !== NOT_PENDING
        );
        if (live.length === 0) carriers.delete(t);
        else carriers.set(t, live);
      }
    if (set.size === 0) overlaidFamilies.delete(fam);
  }
}
/** The node of `t` carrying a lane value, if any — the guess carrier or the
 * deep witness (`read`: one a reader's last pass read). */
function laneNodeOf(t, read) {
  for (const n of [...(kgOf(t) ?? []), t.dk])
    if (n !== null && n._config & CONFIG_OVERRIDE) {
      if (!read) return n;
      for (let s = n._subs; s !== null; s = s._nextSub) if (s._gen === s._sub._depGen) return n;
    }
}
function createOptimisticStore(first, second, third) {
  const derived = typeof first === "function";
  // A derived store's family (projection.ts): the derive's status reaches
  // its leaves' readers as for `createStore(fn)` (F-12, #4437).
  const store = createProjectionInternal(
    derived ? first : null,
    derived ? second : first,
    derived ? third : second,
    true
  );
  return [
    store,
    fn => (
      derived &&
        (devGuardOptimisticBeforeLoad(store[$TARGET]._fam.node),
        devWarnOptimisticWhileLoading(store[$TARGET]._fam.node)),
      storeSetter(store, fn)
    )
  ];
}

function trueFn() {
  return true;
}
/** @internal What a source ENTRY is, decided once when the view is built
 * (`merge()` learns it while flattening; `omit()` from its argument) and
 * carried beside the entry — `MergeView.kinds[i]`, `OmitView.kind` — so no
 * read has to ask. Asking is the cost: any brand check on a Proxy is a trap
 * (`instanceof` is a `getPrototypeOf` trap, ~20 ns on a store — as much as
 * the read itself), and a merge over a store did two per read. */
const SOURCE_PLAIN = 0; // a plain object: own keys fixed, data is data
const SOURCE_OMIT = 1; // an `OmitView` record (only as a merge entry)
const SOURCE_PROXY = 2; // a store or foreign proxy: everything is a trap
const SOURCE_MEMO = 3; // a function source (merge's memo): swaps objects
const SOURCE_MERGE = 4; // a `MergeView` record (only as an omit's source)
const EMPTY = Object.freeze({});
// The object behind a LEAF entry (any kind but OMIT): a memo is read
// (tracked, as merge's own reads are) and a nullish result has no keys.
function leafOf(s, kind) {
  return kind === SOURCE_MEMO ? (s = s()) || EMPTY : s;
}
/** @internal The record behind an `omit()` proxy: `source` with `hidden`
 * keys removed. It is the proxy's TARGET, so the shared handler reads it as
 * plain fields — no per-instance closures — and it is what props consumers
 * walk directly (`merge`, `spread`, `ssrElement`): a view never materializes
 * a copy, and a consumer that knows the record never goes through its traps
 * (a `getOwnPropertyDescriptor` trap per key allocates a descriptor and a
 * getter, so enumerating a proxy costs more than the copy it was avoiding).
 * `hidden` is a key list or a predicate (`omit(props, k => k[0] === "$")`).
 *
 * An omit over a `merge()` holds the merge's RECORD (`MergeView`, kind
 * `SOURCE_MERGE`) — never its proxy, so no read hops through a trap — and is
 * one record however many leaves the merge has. A consumer walks it as ONE
 * filtered entry (`sourceKeys` / `sourceGet` recurse into the merge's
 * sources by function call), and a later `merge()` over it carries the
 * record as one entry instead of copying its leaves: on a component chain of
 * defaults + omit + spread (`merge(omit(merge(omit(props))))`) the layers
 * nest as records, each a few fields, where a flatten to leaf views built a
 * view and a combined key list per leaf per layer — the largest allocation
 * of a Kobalte-shaped render. A merge leaf may be merge's memo for a
 * function source; it is resolved on access. */
class OmitView {
  source;
  kind;
  hidden;
  /** see `MergeView.table` */
  table = 0;
  /** see `tableOwnKeys` / `tableDescriptor` */
  keys = undefined;
  descs = undefined;
  constructor(
    source,
    /** of `source` — PLAIN, PROXY (a store or a foreign proxy), MEMO, or
     * MERGE (a `MergeView` record); never OMIT, a view over a view folds
     * into one. */
    kind,
    hidden
  ) {
    this.source = source;
    this.kind = kind;
    this.hidden = hidden;
  }
}
function isHidden(view, key) {
  const h = view.hidden;
  return typeof h === "function" ? h(key) : h.includes(key);
}
// Both filters as one. Two key lists stay a key list (one `includes`, no
// closure); a predicate on either side needs a closure. An omit over a
// merge builds one combined list per leaf, per component layer, so the
// copy's form matters in every tier: `concat` runs the species/spreadable
// protocol (2–3× the cost of a copy once optimized), a hand loop is 2–4×
// `concat` in the interpreter and baseline tiers (a bytecode per element
// against one builtin), and a presized `new Array(n)` is holey, which takes
// `includes` off its fast path. `slice` + `push` of the (short) second list
// is within a third of the best form in every tier, and packed.
function combineHidden(a, b) {
  if (typeof a !== "function" && typeof b !== "function") {
    const out = a.slice();
    for (let i = 0; i < b.length; i++) out.push(b[i]);
    return out;
  }
  return key =>
    (typeof a === "function" ? a(key) : a.includes(key)) ||
    (typeof b === "function" ? b(key) : b.includes(key));
}
// The object a view filters (see `leafOf`).
function viewSource(view) {
  return leafOf(view.source, view.kind);
}
// The record behind a `$PROXY`-marked object that is one of OUR views, else
// undefined for a store or a foreign proxy. ONE read: the view traps answer
// `$RECORD` first thing and a store's `get` trap answers it `undefined` on
// its symbol fast path, so the question never reaches a store's generic
// read path (firewall gate, tracked key read), which is what an unknown
// symbol would take — and a foreign proxy forwards it to a target that has
// no such key. Asking `$TARGET` first and then each view kind was two to
// four trap hops per source at every `merge()`, `omit()` and `ssrElement`.
// Call only after `$PROXY in o` is known true.
function recordOf(o) {
  return o[$RECORD];
}
/** @internal The `OmitView` behind an `omit()` proxy, or undefined. */
function omitView(o) {
  if (!o || !($PROXY in o)) return undefined;
  const r = o[$RECORD];
  return r instanceof OmitView ? r : undefined;
}
// A props SOURCE ENTRY is a plain object, a proxy (store, merge, omit — the
// last two are normally unwrapped first: `mergeView` / `omitView`), a memo,
// or an `OmitView` record, and travels with its kind. These three answer for
// an entry what `Object.keys` / `in` / `[]` answer for an object, so every
// consumer walks entries with one code path, an `OmitView` is filtered
// rather than materialized, and nothing is asked of a proxy but the read.
// Own keys of a leaf: a proxy answers through ONE `ownKeys` trap (a store's
// keeps the key set tracked; `Object.keys` on a proxy would add a descriptor
// trap per key); a plain object its enumerable string keys. What a memo
// holds is only known once read.
function leafKeys(leaf, kind) {
  if (kind === SOURCE_PLAIN) return Object.keys(leaf);
  if (kind === SOURCE_PROXY || leaf[$PROXY] === leaf) {
    // A store's handler answers directly: the Proxy `ownKeys` invariant checks cost twice the trap.
    const t = leaf[$TARGET];
    return (t?.px === leaf ? t : Reflect).ownKeys(leaf);
  }
  return Object.keys(leaf);
}
/** @internal Own string keys of a source entry — every consumer skips
 * symbols itself. */
function sourceKeys(s, kind) {
  if (kind === SOURCE_OMIT) {
    if (s.kind === SOURCE_MERGE) return mergeKeysOf(s.source, false, s);
    const keys = leafKeys(viewSource(s), s.kind);
    const out = [];
    for (let i = 0; i < keys.length; i++) if (!isHidden(s, keys[i])) out.push(keys[i]);
    return out;
  }
  return leafKeys(leafOf(s, kind), kind);
}
/** @internal `key in entry`. */
function sourceHas(s, kind, key) {
  if (kind === SOURCE_OMIT) {
    if (isHidden(s, key)) return false;
    return s.kind === SOURCE_MERGE ? mergeHas(s.source, key) : key in viewSource(s);
  }
  return key in leafOf(s, kind);
}
/** @internal `entry[key]` — the source's getter runs once, here. */
function sourceGet(s, kind, key) {
  if (kind === SOURCE_OMIT) {
    if (isHidden(s, key)) return undefined;
    return s.kind === SOURCE_MERGE ? mergeGet(s.source, key) : viewSource(s)[key];
  }
  return leafOf(s, kind)[key];
}
// An entry whose own key set is fixed: a plain object, or a view over one —
// an omit of a plain object, or of a merge whose entries all are. Not a
// store (its key set is a tracked signal), not a merge memo source (it
// swaps whole objects), and not any proxy that declares itself with
// `$PROXY in s` — a frames slot proxy answers `has` for every key and lists
// none, so only the `in` walk is right for it.
function entryHasStaticKeys(s, kind) {
  if (kind === SOURCE_PLAIN) return true;
  if (kind !== SOURCE_OMIT) return false;
  if (s.kind === SOURCE_PLAIN) return true;
  return s.kind === SOURCE_MERGE && mergeHasStaticKeys(s.source);
}
function mergeHasStaticKeys(view) {
  const f = view.sources,
    k = view.kinds;
  for (let i = 0; i < f.length; i++) if (!entryHasStaticKeys(f[i], k[i])) return false;
  return true;
}
/** @internal Whether the own key set of a props object cannot change
 * reactively: a plain object, or a merge/omit view over plain objects only.
 * A consumer may then decide from `Object.getOwnPropertyDescriptor` once —
 * "no `children` key" or "a data `children`" holds for the object's lifetime,
 * so no tracking scope is needed for it (#3388). For a store, or a view with
 * a store or memo leaf, keys can appear later and the reactive path is the
 * only correct one. */
function hasStaticKeys(o) {
  if (!($PROXY in o)) return true;
  const r = recordOf(o);
  if (r instanceof MergeView) return mergeHasStaticKeys(r);
  return r !== undefined && entryHasStaticKeys(r, SOURCE_OMIT);
}
/**
 * Whether `o[key]` can never change for the lifetime of `o`: the key is a
 * data property of a plain object, or is absent from an object whose key set
 * is fixed. A getter, a key on a store, a memo-backed `merge()` source, or
 * any key of an object whose keys can appear later (a store) is not static.
 *
 * Looks through `merge()`/`omit()` views to the leaf that owns the key. Any
 * object will do, but props are the case it exists for: the compiler encodes
 * a literal at the call site (`as="button"`) as a data property and an
 * expression (`as={isLink() ? "a" : "button"}`) as a getter, so a component
 * library reads the caller's own static/dynamic classification of a prop at
 * runtime — identically on server and client, the compiler emitting the same
 * own DESCRIPTORS on both (the object behind them may differ: the server
 * builds props as a plain-prototype instance with shared getters) — and can
 * take a no-computation path for the literal:
 *
 * ```tsx
 * const Tag = dynamic(() => props.as, { static: isStatic(props, "as") });
 * ```
 *
 * One descriptor lookup; no read of the value, nothing tracked.
 */
function isStatic(o, key) {
  if ($PROXY in o) {
    // A store answers its descriptor trap with a value; through a view the
    // descriptor is truthful (see `sourceDescriptor`). A foreign proxy is
    // opaque: nothing about it is known to be fixed.
    if (viewOf(o) === undefined) return false;
    const desc = Reflect.getOwnPropertyDescriptor(o, key);
    return desc === undefined ? hasStaticKeys(o) : desc.get === undefined;
  }
  const desc = Reflect.getOwnPropertyDescriptor(o, key);
  return desc === undefined || (desc.get === undefined && desc.set === undefined);
}
function accessorDescriptor(get, enumerable = true) {
  return { configurable: true, enumerable, get, set: trueFn };
}
/** The descriptor a consumer should see for `key` on an entry —
 * the view proxies answer `getOwnPropertyDescriptor` with it, so it tells the
 * truth through any depth of merge/omit layers.
 *
 * A DATA descriptor means "nothing reactive can hide behind this value": the
 * key is a data property of a plain-object leaf — the compiler's own
 * encoding of a static attribute. Everything else is an accessor: a getter
 * on a leaf, a key on a store proxy (its "data" is a signal), or a key on a
 * merge memo source (the whole object is reactive). That is what lets a
 * consumer skip a reactive node for a static prop at the bottom of a
 * component chain, and it is why the store case must NOT forward the store's
 * own descriptor, which reports a value.
 *
 * `configurable: true` always — the target has no such property, and the
 * Proxy invariants forbid reporting a non-configurable one. */
// `present` says the caller has already established `key in s` (a trap's
// shadowing walk did), so a store is not asked a second time.
function sourceDescriptor(s, kind, key, present = false) {
  if (kind === SOURCE_OMIT) {
    if (isHidden(s, key)) return undefined;
    return sourceDescriptor(s.source, s.kind, key, present);
  }
  // An omit's merge record: the descriptor of the last entry that has the
  // key, as the merge proxy's own trap answers (see `mergeDescriptor`).
  if (kind === SOURCE_MERGE) return mergeDescriptor(s, key);
  // A memo source (`merge(() => …)`) is reactive wholesale: whatever shape
  // the memo's current object has, the key is an accessor here.
  if (kind === SOURCE_MEMO) {
    return present || key in leafOf(s, kind)
      ? accessorDescriptor(() => leafOf(s, kind)[key])
      : undefined;
  }
  if (kind === SOURCE_PROXY) {
    // Another view (an omit's source may be a merge proxy) already answers
    // truthfully. A store's reported "data" is a signal, and a foreign proxy
    // (frames slot props) has no own descriptors: for both, existence is
    // `in` and the kind is accessor — one trap, and never the store's
    // descriptor path.
    if (recordOf(s) !== undefined) return Reflect.getOwnPropertyDescriptor(s, key);
    return present || key in s ? accessorDescriptor(() => s[key]) : undefined;
  }
  const desc = Reflect.getOwnPropertyDescriptor(s, key);
  if (desc === undefined) return undefined;
  if (desc.get !== undefined || desc.set !== undefined)
    return accessorDescriptor(() => s[key], desc.enumerable);
  // The proxy target has no such key, so the descriptor must be configurable;
  // Reflect's is a fresh object, so a configurable one is handed out as is.
  if (desc.configurable) return desc;
  return { configurable: true, enumerable: desc.enumerable, writable: true, value: desc.value };
}
// Own ENUMERABLE keys, symbols included, of an entry — the user-facing key
// set (`Object.keys(merged)`), where enumerability matters (#2769).
function sourceEnumerableKeys(s, kind) {
  {
    if (s.kind === SOURCE_MERGE) return mergeEnumerableKeys(s.source, s);
    const keys = ownEnumerableKeys(viewSource(s));
    const out = [];
    for (let i = 0; i < keys.length; i++) if (!isHidden(s, keys[i])) out.push(keys[i]);
    return out;
  }
}
// The target of a merge() proxy: the flattened sources, read by one shared
// handler — like OmitView, no per-instance closures. `sources` is what
// `mergeSources` answers.
/** @internal */
class MergeView {
  sources;
  kinds;
  /** key → the plain leaf that owns it (later sources win), built by an
   * enumeration or once the reads have paid for it (see `resolvedTable`)
   * when every leaf has static keys; `null` when one doesn't. Until then
   * the slot counts the per-key trap reads so far. One slot rather than a
   * counter field of its own: a view is built per source per component
   * layer, and each field initializer is a measurable share of a
   * constructor that small in the lower JIT tiers. */
  table = 0;
  /** see `tableOwnKeys` / `tableDescriptor` */
  keys = undefined;
  descs = undefined;
  constructor(
    sources,
    /** `kinds[i]` is what `sources[i]` is (see `SourceKind`). */
    kinds
  ) {
    this.sources = sources;
    this.kinds = kinds;
  }
}
/** @internal The `MergeView` behind a `merge()` proxy — its flattened
 * `sources` with their `kinds` — or undefined. */
function mergeView(o) {
  if (!o || !($PROXY in o)) return undefined;
  const r = o[$RECORD];
  return r instanceof MergeView ? r : undefined;
}
/** @internal The record behind a merge() or omit() proxy — a `MergeView`
 * (flattened `sources` with their `kinds`) or an `OmitView` — or undefined
 * for anything else (a plain object, a store, a foreign proxy). Two fast
 * traps on a proxy (`has`, then `$RECORD`), none on a plain object. */
function viewOf(o) {
  return o && $PROXY in o ? o[$RECORD] : undefined;
}
/** @internal The resolved key table of a merge/omit view — every own key of
 * the view mapped to the plain object that owns it, in merged order (a key
 * at the position of the last source that carries it, see `tableSet`) — or
 * undefined when it has none: a leaf is a store or a memo source, whose
 * keys can change, or the object is not a view at all.
 *
 * This is the flat object the eager copy used to build, made lazily and
 * without copying: one pass over the leaves' own keys, then every
 * `get`/`has`/descriptor is one lookup plus one read of the owning leaf, and
 * a consumer (`spread` rerunning its effect) walks the table instead of
 * re-deriving shadowing from the leaves each time. Own keys only, as the
 * copy's were: a plain source's key set is fixed once merged (keys added to
 * it later are not seen — the copy didn't see them either).
 *
 * It is built by an ENUMERATION — the `ownKeys` trap, or a consumer asking
 * for it here — or once per-key reads have paid for it (`READS_FOR_TABLE`),
 * not on the first read. A per-key read has a direct answer (a walk of the
 * sources, last to first, one `in` each) whose cost is the source count,
 * while the table's is every key of every leaf, so the walk wins until a
 * view has been read about as many times as it has keys. On the server it
 * never is: a component reads its props a few times, the element enumerates
 * them once through its own source walk, and the view is gone — building on
 * first read there cost a component chain a table per layer (profiled on
 * the Kobalte-shaped chain: a third of SSR time in the table code and its
 * garbage). On the
 * client a view read on every reactive rerun crosses the threshold in its
 * first few updates and is one lookup per read from then on, as before.
 * Once built — by a `spread`, `Object.keys`, `{...props}`, or the count —
 * every trap uses it. */
function resolvedTable(o) {
  if (!o || !($PROXY in o)) return undefined;
  const r = recordOf(o);
  if (r instanceof OmitView) return omitTable(r);
  return r === undefined ? undefined : mergeTable(r);
}
function mergeTable(view) {
  let table = view.table;
  if (typeof table !== "object") {
    // a read count: not decided yet
    const f = view.sources,
      k = view.kinds;
    for (let i = 0; i < f.length; i++) {
      if (!entryHasStaticKeys(f[i], k[i])) {
        view.table = null;
        return undefined;
      }
    }
    table = new Map();
    collectTable(table, view, undefined);
    view.table = table;
  }
  return table === null ? undefined : table;
}
// One pass over a merge record's leaves into `table`, through the nested
// omit-over-merge entries — the filters enclosing the current leaf are the
// `filters` stack — so a component chain builds ONE table at the view that
// asked, not one per layer. A key an entry hides that an EARLIER entry owned
// must stay: a filter applies to its own entry's contribution, not to the
// merge, which is exactly what the stack expresses. Every entry has static
// keys (the caller checked), so a leaf's own keys are the truth.
function collectTable(table, view, filters) {
  const f = view.sources,
    k = view.kinds;
  for (let i = 0; i < f.length; i++) {
    const leaf = f[i];
    if (k[i] === SOURCE_OMIT) {
      if (leaf.kind === SOURCE_MERGE) {
        if (filters === undefined) filters = [leaf];
        else filters.push(leaf);
        collectTable(table, leaf.source, filters);
        filters.pop();
        continue;
      }
      const src = leaf.source;
      const keys = Reflect.ownKeys(src);
      for (let j = 0; j < keys.length; j++) {
        const key = keys[j];
        if (!isHidden(leaf, key) && !hiddenByAny(filters, key)) tableSet(table, key, src);
      }
    } else {
      const keys = Reflect.ownKeys(leaf);
      for (let j = 0; j < keys.length; j++) {
        const key = keys[j];
        if (!hiddenByAny(filters, key)) tableSet(table, key, leaf);
      }
    }
  }
}
function hiddenByAny(filters, key) {
  if (filters !== undefined)
    for (let i = filters.length - 1; i >= 0; i--) if (isHidden(filters[i], key)) return true;
  return false;
}
// Key order is the merged one — every key at the position of the LAST source
// that carries it — the order `ssrElement`'s array form serializes in and the
// eager copy enumerated in, so a spread through a view and a spread over the
// sources emit the same attribute order.
function tableSet(table, key, leaf) {
  if (table.has(key)) table.delete(key);
  table.set(key, leaf);
}
// An omit view's table: its source's (a merge's table, or a plain object's
// own keys) minus the hidden keys. Cached on the record.
function omitTable(view) {
  let table = view.table;
  if (typeof table !== "object") {
    const src = view.source;
    if (view.kind === SOURCE_MERGE) {
      // One pass over the merge's leaves with this filter on the stack —
      // the merge record builds no table of its own for it.
      if (!mergeHasStaticKeys(src)) {
        view.table = null;
        return undefined;
      }
      table = new Map();
      collectTable(table, src, [view]);
    } else if (view.kind === SOURCE_PLAIN) {
      table = new Map();
      const keys = Reflect.ownKeys(src);
      for (let j = 0; j < keys.length; j++) {
        const key = keys[j];
        if (!isHidden(view, key)) table.set(key, src);
      }
    } else {
      // a store or a memo: keys can change, no table
      view.table = null;
      return undefined;
    }
    view.table = table;
  }
  return table === null ? undefined : table;
}
// The user-facing key set of a resolved table: its keys that are enumerable
// on the leaf that owns them (`Object.keys(merged)`, #2769). Fixed, like the
// table, so it is built once per view: an `ownKeys` trap may hand back the
// same array every time (the engine copies it).
const propertyIsEnumerable = Object.prototype.propertyIsEnumerable;
function tableOwnKeys(view, table) {
  let keys = view.keys;
  if (keys === undefined) {
    keys = view.keys = [];
    for (const [key, leaf] of table) if (propertyIsEnumerable.call(leaf, key)) keys.push(key);
  }
  return keys;
}
// The descriptor for a table key — its owning leaf's, truthful (see
// `sourceDescriptor`) — with the shape cached per key so an enumeration
// (`for…in`, `Object.keys`, `{...props}`: a descriptor trap per key, on
// every pass) does not re-read the leaf's descriptor and re-allocate a
// getter each time. An accessor reads live, so its descriptor is reused as
// is; a data descriptor is rebuilt with the current value.
function tableDescriptor(view, table, key) {
  const leaf = table.get(key);
  if (leaf === undefined) return undefined;
  let descs = view.descs;
  if (descs === undefined) descs = view.descs = new Map();
  let cached = descs.get(key);
  if (cached === undefined) {
    cached = sourceDescriptor(leaf, SOURCE_PLAIN, key);
    if (cached === undefined) return undefined;
    descs.set(key, cached);
    return cached;
  }
  if (cached.get !== undefined) return cached;
  return {
    configurable: true,
    enumerable: cached.enumerable,
    writable: cached.writable,
    value: leaf[key]
  };
}
// Per-key trap reads a view takes before building its table, from the
// break-even: a build is ~60 ns per key (an `ownKeys` share, a `has`, a
// `set`), a walk ~20 ns per source (an `in`, a hidden-list check), and a
// leaf carries about five keys — so the table has paid for itself after
// ~15 reads. Measured on the Kobalte-shaped chain: a walk is 2× a lookup at
// depth 1 and up to 10× for a first-source key at depth 7 (a walk through
// seven nested layers, a hidden-list check at each), so a view read on
// every update wants the table; a view read a handful of times (every
// server-side view) never wants it.
const READS_FOR_TABLE = 16;
// The table for a per-key trap: the one a view HAS (built by an enumeration
// or an earlier read, see `resolvedTable`), or the one this read pays for,
// or none — a walk answers. One per view type, so a trap pays no type check.
// A settled slot is an object (the Map, or `null`); a number is the count.
function mergeReadTable(view) {
  const table = view.table;
  if (typeof table === "object") return table === null ? undefined : table;
  if (table + 1 < READS_FOR_TABLE) {
    view.table = table + 1;
    return undefined;
  }
  return mergeTable(view);
}
// Only for an omit over a merge (`kind === SOURCE_MERGE`; the caller checks,
// inline — a call is not free in every tier): an omit over one object reads
// it directly — a list check and a property read, nothing a table would
// shorten.
function omitReadTable(view) {
  const table = view.table;
  if (typeof table === "object") return table === null ? undefined : table;
  if (table + 1 < READS_FOR_TABLE) {
    view.table = table + 1;
    return undefined;
  }
  return omitTable(view);
}
// The table a record HAS — built already by an enumeration or a trap's read
// count — or undefined. What a nested walk asks: a record reached through an
// outer view's entry counts no reads of its own (the outer view decides for
// the whole tree, and its table build then builds the inner ones), so the
// inner merges of a component chain build nothing on the server where the
// leaves are read a few times each.
function tableOf(view) {
  const table = view.table;
  return typeof table === "object" && table !== null ? table : undefined;
}
// "no entry has the key" — distinct from an entry that holds `undefined`.
const MISSING = Symbol();
// The read: the value of the last entry that has the key, or MISSING. ONE
// walk — a nested omit-over-merge entry answers presence and value together,
// so a chain of layers is walked once per read, not once per layer per
// level.
function mergeLookup(view, property) {
  const table = tableOf(view);
  if (table !== undefined) {
    const leaf = table.get(property);
    return leaf === undefined ? MISSING : leaf[property];
  }
  const f = view.sources,
    k = view.kinds;
  for (let i = f.length - 1; i >= 0; i--) {
    const kind = k[i];
    // The common leaf first, read in place: a component's props view is a
    // few plain objects, read once per key on the server.
    if (kind === SOURCE_PLAIN) {
      const s = f[i];
      if (property in s) return s[property];
      continue;
    }
    if (kind === SOURCE_OMIT) {
      const v = f[i];
      if (isHidden(v, property)) continue;
      if (v.kind === SOURCE_MERGE) {
        const value = mergeLookup(v.source, property);
        if (value !== MISSING) return value;
        continue;
      }
      const s = viewSource(v);
      if (property in s) return s[property];
    } else {
      const s = leafOf(f[i], kind);
      if (property in s) return s[property];
    }
  }
  return MISSING;
}
function mergeGet(view, property) {
  const value = mergeLookup(view, property);
  return value === MISSING ? undefined : value;
}
// `key in merge`, on the record.
function mergeHas(view, property) {
  const table = tableOf(view);
  if (table !== undefined) return table.has(property);
  const f = view.sources,
    k = view.kinds;
  for (let i = f.length - 1; i >= 0; i--) if (sourceHas(f[i], k[i], property)) return true;
  return false;
}
// The proxy's `getOwnPropertyDescriptor`, on the record.
function mergeDescriptor(view, property) {
  const table = tableOf(view);
  if (table !== undefined) return tableDescriptor(view, table, property);
  const f = view.sources,
    k = view.kinds;
  for (let i = f.length - 1; i >= 0; i--) {
    if (!sourceHas(f[i], k[i], property)) continue;
    // `in` also answers for inherited keys, which have no own descriptor.
    return (
      sourceDescriptor(f[i], k[i], property, true) ??
      accessorDescriptor(() => mergeGet(view, property))
    );
  }
  return undefined;
}
// Own keys of a merge record in merged order — every key at the position
// of the LAST entry that carries it, the order the table keeps and
// `ssrElement` serializes in. `enumerable` selects the user-facing set
// (`Object.keys`, #2769) over every own string key (a consumer's walk);
// `filter` is the omit this record is read through, applied as the keys
// are gathered so an omit over a merge builds ONE list per layer. A list
// with `indexOf` rather than a Set: a props object has a dozen keys, and a
// Set's hash store was 2 KB per row on the Kobalte-shaped chain.
function mergeKeysOf(view, enumerable, filter) {
  const out = [];
  collectKeys(view, filter === undefined ? undefined : [filter], enumerable, out, null);
  return out;
}
// One pass over a merge record's leaves — through its nested omit-over-merge
// entries, `filters` the omits enclosing the current leaf (see
// `collectTable`) — appending each leaf's keys to `keys` in merged order
// (a key already listed moves to the end: later wins) and, when `owners` is
// given, the object that owns the key at the same index. A consumer that
// reads every key once (`ssrElement`) then reads `owners[i][keys[i]]`: no
// `in` walk per key, no table. A memo leaf is resolved once here.
function collectKeys(view, filters, enumerable, keys, owners) {
  const f = view.sources,
    k = view.kinds;
  for (let i = 0; i < f.length; i++) {
    let leaf = f[i],
      kind = k[i];
    let filter;
    if (kind === SOURCE_OMIT) {
      if (leaf.kind === SOURCE_MERGE) {
        if (filters === undefined) filters = [leaf];
        else filters.push(leaf);
        collectKeys(leaf.source, filters, enumerable, keys, owners);
        filters.pop();
        continue;
      }
      filter = leaf;
      kind = leaf.kind;
      leaf = leaf.source;
    }
    leaf = leafOf(leaf, kind);
    const ks = enumerable ? ownEnumerableKeys(leaf) : leafKeys(leaf, kind);
    for (let j = 0; j < ks.length; j++) {
      const key = ks[j];
      if (filter !== undefined && isHidden(filter, key)) continue;
      if (hiddenByAny(filters, key)) continue;
      addKey(keys, owners, key, leaf);
    }
  }
}
// Append `key` owned by `owner`, moving an earlier listing to the end: later
// wins, and the position is the last owner's (the merged order).
function addKey(keys, owners, key, owner) {
  const at = keys.indexOf(key);
  if (at !== -1) {
    keys.splice(at, 1);
    if (owners !== null) owners.splice(at, 1);
  }
  keys.push(key);
  if (owners !== null) owners.push(owner);
}
/** @internal Every own string key of a props SOURCE — a plain object, a
 * store or foreign proxy, or a merge/omit view — appended to `keys` in
 * merged order with the object that owns each at the same index of
 * `owners`: a key already listed (by this source or an earlier one) moves
 * to the end, so several sources collected in turn give the order and the
 * winners a merge of them would. A consumer that reads each key once
 * (`ssrElement`) then reads `owners[i][keys[i]]` — the owner's getter runs
 * there, once — and asks nothing else of a view: no table, no `in` walk per
 * key through the merge/omit layers, no key list per leaf. One pass,
 * however deep the layers nest. Symbols are listed; the consumer skips
 * them. */
function sourceOwners(s, keys, owners) {
  if ($PROXY in s) {
    const view = viewOf(s);
    if (view === undefined) {
      // a store: one `ownKeys` trap, reads through `[]`
      const ks = Reflect.ownKeys(s);
      for (let i = 0; i < ks.length; i++) addKey(keys, owners, ks[i], s);
      return;
    }
    if (view instanceof OmitView) {
      if (view.kind === SOURCE_MERGE) return collectKeys(view.source, [view], false, keys, owners);
      const leaf = viewSource(view);
      const ks = leafKeys(leaf, view.kind);
      for (let i = 0; i < ks.length; i++)
        if (!isHidden(view, ks[i])) addKey(keys, owners, ks[i], leaf);
      return;
    }
    return collectKeys(view, undefined, false, keys, owners);
  }
  const ks = Object.keys(s);
  for (let i = 0; i < ks.length; i++) addKey(keys, owners, ks[i], s);
}
// The user-facing key set of a merge record, read through `filter` if given.
function mergeEnumerableKeys(view, filter) {
  const table = mergeTable(view);
  if (table === undefined) return mergeKeysOf(view, true, filter);
  const keys = tableOwnKeys(view, table);
  if (filter === undefined) return keys;
  const out = [];
  for (let i = 0; i < keys.length; i++) if (!isHidden(filter, keys[i])) out.push(keys[i]);
  return out;
}
const mergeTraps = {
  get(view, property, receiver) {
    // The private keys are symbols; a string read (every prop) skips the
    // compares. `$TARGET` is answered so a store check never walks the
    // sources (a leaf that is a store would answer it).
    if (typeof property === "symbol") {
      if (property === $PROXY) return receiver;
      if (property === $RECORD) return view;
      if (property === $TARGET) return undefined;
    }
    // A trap read counts toward the table (see `mergeReadTable`); the walk
    // itself is the record's. `mergeReadTable` and the plain walk of
    // `mergeLookup` are inlined here: a trap is entered from the runtime, so
    // nothing below it is inlined for it, and a component's props view is a
    // few plain objects read once per key on the server — the walk is the
    // whole read. A leaf that is not plain hands the walk to `mergeGet`,
    // which starts over (a plain leaf walked twice is two `in` checks).
    const state = view.table;
    let table;
    if (typeof state !== "object") {
      if (state + 1 < READS_FOR_TABLE) {
        view.table = state + 1;
        const f = view.sources,
          k = view.kinds;
        for (let i = f.length - 1; i >= 0; i--) {
          if (k[i] !== SOURCE_PLAIN) return mergeGet(view, property);
          // Read first, `in` only to tell a missing key from one holding
          // undefined: the last source is the one that usually has the key.
          const v = f[i][property];
          if (v !== undefined || property in f[i]) return v;
        }
        return undefined;
      }
      table = mergeTable(view);
      if (table === undefined) return mergeGet(view, property);
    } else if (state === null) return mergeGet(view, property);
    else table = state;
    const leaf = table.get(property);
    return leaf === undefined ? undefined : leaf[property];
  },
  has(view, property) {
    if (property === $PROXY) return true;
    if (property === $TARGET || property === $RECORD) return false;
    const table = mergeReadTable(view);
    if (table !== undefined) return table.has(property);
    return mergeHas(view, property);
  },
  set: trueFn,
  deleteProperty: trueFn,
  getOwnPropertyDescriptor(view, property) {
    if (property === $PROXY || property === $TARGET || property === $RECORD) return undefined;
    const table = mergeReadTable(view);
    if (table !== undefined) return tableDescriptor(view, table, property);
    return mergeDescriptor(view, property);
  },
  ownKeys(view) {
    return mergeEnumerableKeys(view);
  }
};
// An omit view reads its source directly — a hidden-key check and one
// property read; over a merge record, the merge's own walk by function call,
// never a trap. Once an enumeration or the read count has built its table
// (over a merge with plain leaves only: the merge's, filtered) every trap
// answers from that instead.
const omitTraps = {
  get(view, property, receiver) {
    if (property === $PROXY) return receiver;
    // `$RECORD` is THIS view — never the underlying merge's record, which is
    // unfiltered: handing a re-merge the merge's own sources would leak the
    // omitted keys (#3014). A consumer walks the omit record as ONE filtered
    // entry (`recordOf` tells the two records apart by class).
    if (property === $RECORD) return view;
    if (property === $TARGET) return undefined;
    if (view.kind === SOURCE_MERGE) {
      const table = omitReadTable(view);
      if (table !== undefined) {
        const leaf = table.get(property);
        return leaf === undefined ? undefined : leaf[property];
      }
      if (isHidden(view, property)) return undefined;
      return mergeGet(view.source, property);
    }
    if (isHidden(view, property)) return undefined;
    return viewSource(view)[property];
  },
  has(view, property) {
    if (property === $PROXY) return true;
    if (property === $TARGET || property === $RECORD) return false;
    if (view.kind === SOURCE_MERGE) {
      const table = omitReadTable(view);
      if (table !== undefined) return table.has(property);
      if (isHidden(view, property)) return false;
      return mergeHas(view.source, property);
    }
    if (isHidden(view, property)) return false;
    return property in viewSource(view);
  },
  set: trueFn,
  deleteProperty: trueFn,
  getOwnPropertyDescriptor(view, property) {
    if (property === $PROXY || property === $TARGET || property === $RECORD) return undefined;
    if (view.kind === SOURCE_MERGE) {
      const table = omitReadTable(view);
      if (table !== undefined) return tableDescriptor(view, table, property);
    }
    return sourceDescriptor(view, SOURCE_OMIT, property);
  },
  ownKeys(view) {
    if (view.kind === SOURCE_MERGE) {
      const table = omitTable(view);
      if (table !== undefined) return tableOwnKeys(view, table);
      // No table (a store or memo leaf): the merge's own key set, filtered.
      return sourceEnumerableKeys(view);
    }
    const keys = Reflect.ownKeys(viewSource(view));
    const out = [];
    for (let i = 0; i < keys.length; i++) if (!isHidden(view, keys[i])) out.push(keys[i]);
    return out;
  }
};
/** @internal The flattened sources behind a `merge()` proxy, or undefined.
 * A merge's writes are no-ops, so its sources are the whole truth. A COPY of
 * a merge (`{...merged}`, a descriptor copy) is a plain object that carries
 * no sources — `ownKeys` never lists $RECORD — so what is on the copy is
 * the truth there and every consumer reads it directly (#3384). */
function mergeSources(o) {
  const r = mergeView(o);
  return r === undefined ? undefined : r.sources;
}
/**
 * Merges multiple props-like objects into a single proxy that *preserves
 * reactivity*. Reads are forwarded to the right-most source that defines the
 * property, so later sources override earlier ones (like `Object.assign`).
 *
 * Function arguments are treated as memo-backed sources — useful for passing
 * derived defaults whose computation should track reactively.
 *
 * The result is a live VIEW of its sources, never a copy: creating it costs
 * nothing per key, every read goes to the source that owns the key (a getter
 * runs there, a data property is read live), and writing to it is a no-op.
 * A single non-function source is returned as is. To own a mutable object,
 * copy it: `{ ...merged }` snapshots the current values.
 *
 * Use this in component bodies to merge defaults / overrides without losing
 * Solid's per-property tracking.
 *
 * Reading props, here and anywhere: a prop's getter is defined only for a
 * read through its own object (`props.x`, `{ ...props }`, `Reflect.get`,
 * these views). Forwarding its descriptor onto another object and reading it
 * there is not supported — the compiler's server-side props keep their state
 * on the instance, so the getter needs its object as receiver. A copy that
 * must stay live defines its own getter that reads through the source, as
 * the no-Proxy paths of merge() and omit() do.
 *
 * @example
 * ```tsx
 * function Button(_props: { label: string; type?: string; disabled?: boolean }) {
 *   const props = merge({ type: "button", disabled: false }, _props);
 *
 *   return <button type={props.type} disabled={props.disabled}>{props.label}</button>;
 * }
 * ```
 */
function merge(...sources) {
  if (sources.length === 1 && typeof sources[0] !== "function") return sources[0];
  // Sized to the argument count up front: a view is built per source per
  // component layer, and growing two empty arrays by `push` was a third of
  // its construction. A nested view's sources may run past the count (the
  // array grows), a falsy source leaves it short (trimmed below).
  const flattened = new Array(sources.length);
  const kinds = new Array(sources.length);
  let n = 0;
  // The one non-falsy source, if there is exactly one: it IS the merge.
  let only = undefined;
  let count = 0;
  for (let i = 0; i < sources.length; i++) {
    const s = sources[i];
    if (!s) continue;
    count++;
    only = s;
    if (typeof s === "function") {
      // Observe tiers name it and exempt it from UNSTABLE_MEMO_OUTPUT (see
      // CONFIG_KEY_BAG); prod passes no options.
      flattened[n] = createMemo(s, { name: "merge", _extraConfig: CONFIG_KEY_BAG });
      kinds[n++] = SOURCE_MEMO;
      continue;
    }
    if ($PROXY in s) {
      // A store (or a foreign proxy) is a leaf as it is. A merge() proxy is
      // flattened through: its writes are no-ops, so its sources are exactly
      // what it reads. An omit() proxy joins as its view record — ONE entry,
      // its filter travelling with it, whether it is over a plain object or
      // a whole merge (never the merge's own sources, which would leak the
      // omitted keys, #3014). A consumer's walk recurses into the record.
      const r = recordOf(s);
      if (r instanceof MergeView) {
        for (let j = 0; j < r.sources.length; j++) {
          flattened[n] = r.sources[j];
          kinds[n++] = r.kinds[j];
        }
      } else if (r !== undefined) {
        flattened[n] = r;
        kinds[n++] = SOURCE_OMIT;
      } else {
        flattened[n] = s;
        kinds[n++] = SOURCE_PROXY;
      }
      continue;
    }
    flattened[n] = s;
    kinds[n++] = SOURCE_PLAIN;
  }
  if (n !== flattened.length) {
    flattened.length = n;
    kinds.length = n;
  }
  if (SUPPORTS_PROXY) {
    if (count === 1 && typeof only !== "function") return only;
    // Always a view, never a copy. Building a plain object here costs a
    // descriptor read, a bound getter and a defineProperty per key per
    // layer, and component libraries stack several layers per element
    // (defaults → omit → call-site statics → …), so the copies dominated
    // their render cost while every consumer that matters — `spread`,
    // `ssrElement`, a nested merge — reads the flattened sources directly
    // anyway (#3448). The view is O(1) to create and reads through to the
    // sources, so a data property on a source is read live, like a getter.
    // Writes to the result are no-ops (a consumer that needs its own object
    // copies: `{...merged}`, which the traps answer truthfully). Copies of
    // the result never carry $RECORD (#3384): `ownKeys` answers only the
    // sources' keys.
    return new Proxy(new MergeView(flattened, kinds), mergeTraps);
  }
  // No Proxy: an eager descriptor copy, semantics as close to the view as a
  // plain object allows (getters stay live; data properties are snapshots).
  const defined = Object.create(null);
  let nonTargetKey = false;
  let lastIndex = flattened.length - 1;
  for (let i = lastIndex; i >= 0; i--) {
    const source = flattened[i];
    if (!source) {
      i === lastIndex && lastIndex--;
      continue;
    }
    const sourceKeys = Object.getOwnPropertyNames(source);
    for (let j = sourceKeys.length - 1; j >= 0; j--) {
      const key = sourceKeys[j];
      if (key === "__proto__" || key === "constructor") continue;
      if (!defined[key]) {
        nonTargetKey = nonTargetKey || i !== lastIndex;
        const desc = Object.getOwnPropertyDescriptor(source, key);
        defined[key] = desc.get
          ? {
              enumerable: true,
              configurable: true,
              get: desc.get.bind(source)
            }
          : desc;
      }
    }
  }
  if (!nonTargetKey) return flattened[lastIndex];
  const target = {};
  const definedKeys = Object.keys(defined);
  for (let i = definedKeys.length - 1; i >= 0; i--) {
    const key = definedKeys[i],
      desc = defined[key];
    if (desc.get) Object.defineProperty(target, key, desc);
    else target[key] = desc.value;
  }
  return target;
}
function omit(props, ...keys) {
  let hidden = keys.length === 1 && typeof keys[0] === "function" ? keys[0] : keys;
  if (SUPPORTS_PROXY) {
    // A view over a view folds: one record, both filters, the original
    // source — so a consumer walks the real object however deep the omits go.
    // Over a merge() proxy the source is the merge's RECORD (see OmitView):
    // one record whatever the leaf count, read by function call.
    let source = props;
    let kind = SOURCE_PLAIN;
    if (typeof props === "function") kind = SOURCE_MEMO;
    else if ($PROXY in props) {
      kind = SOURCE_PROXY;
      const r = recordOf(props);
      if (r instanceof OmitView) {
        source = r.source;
        kind = r.kind;
        hidden = combineHidden(r.hidden, hidden);
      } else if (r !== undefined) {
        source = r;
        kind = SOURCE_MERGE;
      }
    }
    return new Proxy(new OmitView(source, kind, hidden), omitTraps);
  }
  const result = {};
  const propNames = Object.getOwnPropertyNames(props);
  const isHiddenKey =
    typeof hidden === "function"
      ? hidden
      : hidden.length > 4 && propNames.length > hidden.length
        ? (
            blocked => key =>
              blocked.has(key)
          )(new Set(hidden))
        : key => hidden.includes(key);
  for (const propName of propNames) {
    if (!isHiddenKey(propName)) {
      const desc = Object.getOwnPropertyDescriptor(props, propName);
      if (!desc.get && !desc.set && desc.enumerable && desc.writable && desc.configurable) {
        result[propName] = desc.value;
      } else if (desc.get || desc.set) {
        // An accessor is re-homed with its source as receiver, never copied
        // as-is: a props getter is only defined for a read THROUGH its own
        // object (the compiler's server props keep their state on the
        // instance, so a forwarded descriptor read on the copy throws). Same
        // rule as merge()'s copy path above.
        Object.defineProperty(result, propName, {
          enumerable: desc.enumerable,
          configurable: true,
          get: desc.get && desc.get.bind(props),
          set: desc.set && desc.set.bind(props)
        });
      } else Object.defineProperty(result, propName, desc);
    }
  }
  return result;
}

function createStore(first, second, third) {
  if (typeof first === "function") return createStoreDerived(first, second, third);
  return createStore$1(first, second);
}
/**
 * A setter transform that reconciles `value` into the store by key
 * (`"id"` by default; `null` positional), preserving store identity for
 * surviving rows.
 */
function reconcile(value, key = "id") {
  return state => reconcileState(value, state, key);
}
/** Non-tracking snapshot of a store's current value (source identity for
 * unmodified subtrees). */
function snapshot(value) {
  return snapshot$1(value);
}
/** Tracking snapshot: subscribes to every reachable record. */
function deep(value) {
  return deep$1(value);
}

// The mark core: a declared not-final state on a node that reads pending
// (`isPending` → true) on the node and everything derived from it, without
// holding anything or entangling a reader. Two declarers share it:
// `affects()` (affects.ts — a mark scoped to a transaction, with a store
// half) and `createDeferred` (deferred.ts — a mark whose lifetime is the
// node's own flight). Split out so the second does not ship the first.
//
// Representation: a count on the marked node (`_x._marks`). Coverage of the
// derivations is pull-derived at probe time — `isPending`'s read walks the
// probed node's dependencies for a marked one (verdict.ts → `_marked`) — so
// nothing is stored downstream and a mid-window recompute strands nothing.
// The push half is the verdict readers' re-derivation when a node's marks
// start and end (`repoll`): the only notification a mark makes.
/** Live marks, every declarer — the gate on the probe's walk. */
let active = 0;
/** The verdict readers downstream of `node` — through derivations, stopping
 * at effects — re-derive: the mark's one notification (a probe's answer
 * changed with no value notifying anyone). Truth readers (`until`, the
 * `refresh` waiter) too, which a lagging flight parked (deferred.ts) — but
 * not through a derived store, whose leaves change only once its derive
 * re-runs: they re-derive at that write. */
function repoll(node, seen = new Set(), readers = CONFIG_VERDICT | CONFIG_AUTHORITATIVE) {
  for (let s = node._subs; s !== null; s = s._nextSub) {
    const sub = s._sub;
    if (seen.has(sub)) continue;
    seen.add(sub);
    if (sub._config & readers) enqueueSub(sub);
    if (!sub._type) repoll(sub, seen, readers);
    // A derived store's derive: its leaves' readers derive from it too
    // (store/projection.ts; a live target's property, has and keys nodes).
    const fam = sub._fam;
    if (fam)
      for (const t of fam.live)
        for (const o of [t.n, t.h, { k: t.k, d: t.dk }])
          if (o) for (const k of Reflect.ownKeys(o)) o[k] && repoll(o[k], seen, CONFIG_VERDICT);
  }
}
/** A node is covered by a live mark iff it carries one, or derives — through
 * its current dependencies — from one that does. A store slot derives from
 * its family's derive (a derived store's leaves are not the derive's
 * dependents, store/projection.ts), so coverage hops there. A real error
 * outranks an inherited mark (D-23): coverage does not flow through an
 * errored node (a direct mark on one still reads pending). Mid-recompute,
 * only the validated prefix of the dependency list is this pass's. */
function marked(el, seen) {
  if (el._x !== null && el._x._marks !== 0) return true;
  const c = el;
  if (c._statusFlags & STATUS_ERROR || seen.has(el)) return false;
  seen.add(el);
  if (c._config & CONFIG_SLOT_NODE) {
    const derive = c._host?._fam?.node;
    return derive != null && marked(derive, seen);
  }
  const tail = c._flags & REACTIVE_RECOMPUTING_DEPS ? c._depsTail : undefined;
  if (tail !== null)
    for (let d = c._deps ?? null; d !== null; d = d._nextDep) {
      if (marked(d._dep, seen)) return true;
      if (d === tail) break;
    }
  return false;
}
/** One mark on `node`. The declarer repolls once its bookkeeping is done. */
function mark(node) {
  ext(node)._marks++;
  active++;
}
/** Drops one mark from `node`: true when it was the last — the node's
 * verdict readers have re-derived. */
function unmark(node) {
  active--;
  if (--node._x._marks !== 0) return false;
  repoll(node);
  return true;
}
GlobalQueue._marked = el => active !== 0 && marked(el, new Set());
GlobalQueue._mark = mark;

/**
 * Store — the store half of `affects()` (plan sec. 29, plan sec. 37; A24 (4), #2882,
 * #2893, #2904).
 *
 * A mark on a store is a mark on nodes (affects.ts: a count on the node, a
 * dependency walk at probe time): `affects(record, key)` marks the slot's
 * leaf; `affects(record)` marks a carrier (the record's `$AFFECTS` leaf)
 * and every live node in the record's subtree — the edges existing readers
 * subscribed through. Coverage is by RAW IDENTITY, not read path (#2882: a
 * captured row proxy, a derived store sharing the source's raw, never read
 * through the declaration's proxy): the identities reachable at the
 * declaration are the mark's `scope`, so a node born inside the window on
 * a covered record inherits the mark (released with the carrier's last),
 * and an untracked probe through a record with no nodes is witnessed into
 * the verdict. A mark declared on a draft (inside a setter) or over an
 * optimistic family walks the view the writer sees.
 *
 * Imported by `affects()`, which installs it on its first store target
 * (#3891, after #3912): a program with stores that never declares a mark
 * carries none of it. It never imports store.ts — the engine's top level —
 * so a program declaring marks without stores carries this module alone:
 * the store's nodes and its wrappability test come through `GlobalQueue`
 * (`_storeNode`, `_storeWrappable`), and node birth and the untracked traps
 * reach back through `affectsHooks` (hooks.ts).
 */
/** The keyless mark's carrier key: a leaf of the record no read serves. */
const $AFFECTS = Symbol("STORE_AFFECTS");
const scopes = new Map();
function scopeOf(carrier, key) {
  let entry = scopes.get(carrier);
  if (entry === undefined)
    scopes.set(carrier, (entry = { _scope: new Set(), _inherited: [], _key: key }));
  return entry;
}
/** The nodes an `affects(store[, key])` declaration marks (affects.ts
 * registers each). */
function storeMarks(t, key) {
  const node = GlobalQueue._storeNode(t, key === undefined ? $AFFECTS : key);
  const entry = scopeOf(node, key);
  if (key === undefined) {
    const found = [node];
    walk(t.px, entry, found, t._fam, new Set());
    return found;
  }
  entry._scope.add(t.v);
  if (t.pb !== null) entry._scope.add(t.pb);
  return [node];
}
/** The identities reachable from `value` into the scope (the draft's and
 * the committed backing both, when a draft is open; an optimistic family's
 * view), every live node under each record into `found`. Untracked by
 * construction: raws, never traps. */
function walk(value, entry, found, fam, visited) {
  if (!GlobalQueue._storeWrappable(value)) return;
  let t = value[$TARGET];
  // Another store's proxy the record holds (§7b): the mark covers this
  // store's view of it — the chained target, keyed by that proxy — and never
  // that store's own records: a derived store's work cannot change its
  // input's data (#4914). Its children are walked as that store's proxies,
  // the identities this store's chained children are keyed by.
  if (t !== undefined && fam !== null && t._fam !== fam && t.px === value) {
    if (visited.has(value)) return;
    visited.add(value);
    entry._scope.add(value);
    const ct = lookupTarget(value, fam);
    if (ct !== undefined) nodesOf(ct, found);
    const raw = t.pb ?? t.v;
    for (const k of Reflect.ownKeys(raw)) {
      const v = Object.getOwnPropertyDescriptor(raw, k).value;
      const ot = GlobalQueue._storeWrappable(v) ? lookupTarget(v, t._fam) : undefined;
      if (ot !== undefined) walk(ot.px, entry, found, fam, visited);
    }
    return;
  }
  t ??= lookupTarget(value, fam);
  let raw = t !== undefined ? (t.pb ?? t.v) : value;
  if (visited.has(raw)) return;
  visited.add(raw);
  entry._scope.add(raw);
  if (t !== undefined) {
    if (t.pb !== null) entry._scope.add(t.v);
    // The writer's view: the tick's own unflushed guesses are in motion too.
    if (optHooks !== null && t.o) raw = optHooks.view(t, raw, true);
    nodesOf(t, found);
    fam = t._fam ?? fam;
  }
  for (const k of Reflect.ownKeys(raw))
    if (k !== $OWNER) {
      const d = Object.getOwnPropertyDescriptor(raw, k);
      if (d.get === undefined) walk(d.value, entry, found, fam, visited);
    }
}
/** Every live node of `t` into `found`. */
function nodesOf(t, found) {
  const nodes = t.n;
  if (nodes !== null)
    for (const k of Reflect.ownKeys(nodes))
      // Another mark's carrier is its own channel: counting it here would
      // extend that scope's lifetime to this declaration's.
      if (k !== $AFFECTS) found.push(nodes[k]);
  const has = t.h;
  if (has !== null) for (const k of Reflect.ownKeys(has)) found.push(has[k]);
  if (t.k !== null) found.push(t.k);
  if (t.dk !== null) found.push(t.dk);
}
/** A live scope covering `t`'s identity (keyed: for `key`). Chained
 * backings (§7b): a view's backing is another store's proxy — marks cover
 * the BASE raw, so every identity along the chain is checked. */
function covering(t, key, skip) {
  for (const [carrier, entry] of scopes) {
    if (carrier === skip || (entry._key !== undefined && entry._key !== key)) continue;
    for (let r = t.v; ;) {
      if (entry._scope.has(r)) return carrier;
      const inner = r?.[$TARGET];
      if (inner === undefined) break;
      const backing = inner.pb ?? inner.v;
      if (backing === r) break;
      r = backing;
    }
  }
  return null;
}
/** The store's side: node birth and the untracked traps call it (store.ts,
 * through `affectsHooks`) — with no live mark too: an empty scope map is
 * no work. */
const hooks = {
  // A node born on a record a live mark covers inherits it (released with
  // the carrier's last registration).
  _born(t, node, key) {
    if (scopes.size === 0 || key === $AFFECTS) return;
    const carrier = covering(t, key, node);
    if (!carrier) return;
    GlobalQueue._mark(node);
    scopes.get(carrier)._inherited.push(node);
  },
  // An untracked probe (`isPending(() => s.x)` with no observer) reading a
  // record a live mark covers: no node carries the mark for it — the
  // verdict is told directly.
  _witness(t, key) {
    if (t.n?.[$AFFECTS]?._x?._marks || covering(t, key)) GlobalQueue._witnessMark();
  }
};
/** `affects()` calls it before a store-targeted declaration (idempotent). */
function installStoreAffects() {
  installAffectsHooks(hooks);
}
/** The carrier's last mark released (affects.ts): its scope dies, and the
 * nodes that inherited the mark release theirs. */
function releaseMarkScope(carrier) {
  const entry = scopes.get(carrier);
  if (entry === undefined) return;
  scopes.delete(carrier);
  releaseMarksHook(entry._inherited);
}

// `affects()` — the declaration verb of the pending model (A24 (4), ruled
// 2026-07-13; the #2893 audit corollaries; rebuilt on L2 2026-10-02 as ruled:
// transaction-inert).
//
// A mark is a declared in-flight change: the marked node — and everything
// derived from it — reads pending (`isPending` → true) from the declaration
// until the surrounding transaction lands (an ambient mark, declared outside
// one, releases at the next seam; either, on a node whose own flight is
// still up then, at the seam after that flight lands). The marked values stay readable: no reader
// suspends on a mark, a mark holds nothing and entangles nothing — it is not
// a node of its transaction (`blocked` never sees it), and a reader of marked
// data joins nothing through it. A mark is never a re-ask, so a declared
// reload's own `refresh()` cannot silence it; a real error outranks it.
//
// Representation: the mark core (affects-marks.ts) — a count on the marked
// node, coverage pull-derived at probe time, readers repolled at registration
// and release. Here: each mark's scope (the transaction's `_marks`, or the
// ambient list) and the store half.
//
// Installed on `GlobalQueue` when imported: a program that never declares a
// mark carries none of this.
/** Marks declared outside a transaction: released at the next seam. */
const ambient = [];
/** Releases one scope's marks (a landed transaction's, the ambient list at
 * the seam): a node whose last mark drops re-derives its verdict readers. */
function release$1(nodes) {
  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    // A store mark's carrier: its scope (and the marks the nodes born in
    // its window inherited) goes with the last mark.
    if (unmark(n)) releaseMarkScope(n);
  }
  nodes.length = 0;
}
/** One registration: a count on the node, listed with the scope — the
 * flush's transaction, or the ambient list (released at the seam). */
function register(node) {
  mark(node);
  const t = flushTransaction;
  if (t === null) ambient.push(node);
  else {
    const u = resolveTx(t);
    (u._marks ??= []).push(node);
  }
  repoll(node);
}
function affects(target, key) {
  if (arguments.length > 2)
    invalid(
      "affects() takes a single optional key — extra keys are not a path. Mark each slot " +
        'with its own affects(record, key) call, or pass the nested record itself: affects(state.user, "name").'
    );
  // A store (the store half, store/affects.ts): the slot's leaf, or the
  // record's carrier and every live node under it.
  const t = target?.[$TARGET];
  if (t !== undefined) {
    installStoreAffects();
    const nodes = storeMarks(t, key);
    for (let i = 0; i < nodes.length; i++) register(nodes[i]);
    schedule();
    return;
  }
  const node = target?.[$REFRESH];
  if (!node) {
    invalid(
      "affects() expects a Solid source accessor or a store. Pass the store proxy (optionally " +
        "with a property key) or the original accessor, not a wrapper function or an already-read value."
    );
    return;
  }
  if (key !== undefined)
    invalid(
      "affects() keys are only valid on store targets. An accessor is a single slot — pass it " +
        "alone, or target the store record that owns the property."
    );
  register(node);
  schedule();
}
function invalid(detail) {
  const message = "[INVALID_AFFECTS_TARGET] " + detail;
  emitDiagnostic({ code: "INVALID_AFFECTS_TARGET", kind: "write", severity: "error", message });
  throw new Error(message);
}
installMarkHooks(
  nodes => {
    // A landing's mark on a node whose own flight is up (a declared reload's
    // quiet re-ask the action did not wait for) stays to the seam after
    // that flight lands, as an ambient one does: the reload's window is the
    // flight's (#4349). The rest release with the transaction.
    const done = [];
    for (let i = 0; i < nodes.length; i++) (inFlight(nodes[i]) ? ambient : done).push(nodes[i]);
    nodes.length = 0;
    release$1(done);
  },
  parked => {
    if (ambient.length === 0) return;
    // The flush parked: the declaration's window is the frame's — the marks
    // are its transaction's ("nothing async below ⇒ no window").
    if (parked !== null) {
      const list = (parked._marks ??= []);
      for (let i = 0; i < ambient.length; i++) list.push(ambient[i]);
      ambient.length = 0;
      return;
    }
    // Nothing parked, but a marked node's own flight is up (a declared
    // reload's quiet re-ask — the node's, or a store leaf's family derive):
    // the window is the flight's; the mark stays to the seam after the
    // landing. The rest release: verdict-only, nothing to show.
    let kept = 0;
    const done = [];
    for (let i = 0; i < ambient.length; i++) {
      const n = ambient[i];
      if (inFlight(n)) ambient[kept++] = n;
      else done.push(n);
    }
    ambient.length = kept;
    if (done.length !== 0) release$1(done);
  }
);
function inFlight(n) {
  return (
    (n._statusFlags & STATUS_PENDING) !== 0 ||
    ((n._config & CONFIG_SLOT_NODE) !== 0 && GlobalQueue._slotFlight?.(n) === true)
  );
}

// `createDeferred` — an async memo that may lag, but never leads (D-40,
// docs/create-deferred.md).
//
// Once the node has a committed value, its own non-final work is invisible
// to the graph: its readers keep the committed value and hold nothing, its
// landings are changes of their own, and only the verdict (`isPending`) and
// the truth readers (`until`, `refresh`'s waiter) see the flight.
//
// Built from parts the engine already has, so core pays nothing for it:
//
// - **Read side: the loading window** (`_loading`, async.ts). Every pass of
//   a node with a committed value opens the window before it asks, so a
//   flight serves the committed value instead of throwing, and an unready
//   source parks the node instead of pending it. `CONFIG_REDERIVE` makes a
//   source going pending a question for the node's pass (which parks) rather
//   than a status pushed through it to its readers.
// - **Verdict: a mark** (affects-marks.ts) whose lifetime is the flight —
//   started when a pass asks a new question (a `refresh()` re-ask is quiet, F-9),
//   dropped at the landing, the rejection, a sync answer or the node's death.
// - **Never leads:** a flight that lands while an input it read is held
//   derived from that hold's world, so it lands into the hold (`holdNode`;
//   `setSignal` stages it there). An input that changed after the pass
//   would have superseded the flight, so a held input at the landing is one
//   the pass read held.
// - **Truth readers** wait the flight out (`GlobalQueue._lagging`, signals.ts
//   `watch`): `await refresh(d)` delivers the landing, `until` sees only
//   settled values.
/** Deferred nodes with a flight up. */
const lagging = new Set();
/** The deferred node with a flight up that `c`'s value derives from, if
 * any: `c` itself, or one reached through its current dependencies. */
function lagOf(c, seen) {
  if (lagging.has(c)) return c;
  if (seen.has(c)) return;
  seen.add(c);
  if (c._config & CONFIG_SLOT_NODE) return lagOf(c._host?._fam?.node ?? c, seen);
  const tail = c._flags & REACTIVE_RECOMPUTING_DEPS ? c._depsTail : undefined;
  if (tail !== null)
    for (let d = c._deps ?? null; d !== null; d = d._nextDep) {
      const l = lagOf(d._dep, seen);
      if (l) return l;
      if (d === tail) break;
    }
}
GlobalQueue._lagging = c => (lagging.size !== 0 ? lagOf(c, new Set()) : undefined);
/**
 * Creates an async memo that may lag, but never leads. Before its first
 * answer it is an ordinary async memo: readers suspend to the nearest
 * `<Loading>`. After it, a refetch holds nothing: readers keep the last
 * committed value until the new one lands, and the rest of the screen moves
 * on without it. `isPending` reports the refetch, on the node and on what
 * derives from it. Use it for independent widgets (dashboards, feeds,
 * search results) where the previous answer with a "refreshing" indicator is
 * better than waiting with everything else.
 *
 * - It never shows a value ahead of a held change: a refetch that read a
 *   held value lands with that hold.
 * - A rejected refetch errors like any memo; the stale value never masks it.
 * - `refresh(d)` is a quiet re-ask, and `await refresh(d)` and `until`
 *   wait for the landing rather than the stale value.
 * - Combine with `loadingValue` for a node that never suspends at all.
 *
 * @example
 * ```tsx
 * const rows = createDeferred(() => fetchRows(store.period));
 *
 * <section class={{ stale: isPending(() => rows()) }}>
 *   <For each={rows()}>{row => <Row row={row} />}</For>
 * </section>
 * ```
 */
function createDeferred(fn, options) {
  let node;
  /** The flight in force: a promise, a stream, or the error an unready
   * source parked the node with. */
  let flight = null;
  let marked = false;
  let answered = false;
  /** The flight `r` is over. An async answer (`held`) arriving while an
   * input is held lands into that hold first. */
  const land = (r, held) => {
    if (r === null || flight !== r) return;
    flight = null;
    lagging.delete(node);
    if (held)
      for (let d = node._deps; d !== null; d = d._nextDep)
        if (d._dep._config & CONFIG_HELD) {
          holdNode(node, txOf(d._dep));
          break;
        }
    if (marked) {
      marked = false;
      unmark(node);
      schedule();
    }
  };
  const ask = r => {
    flight = r;
    lagging.add(node);
    if (!marked && !(node._flags & REACTIVE_REASK)) {
      marked = true;
      mark(node);
      repoll(node);
    }
  };
  /** Registers the flight and serves the pass's value: the committed one,
   * or over a held landing the landing itself, which a lagging question
   * does not replace (D-40) — restaged as the committed value, the hold's
   * readers would re-derive against the old input. */
  const serve = r => {
    const v = handleAsync(node, r);
    return node._config & CONFIG_HELD && node._pendingValue !== NOT_PENDING
      ? node._pendingValue
      : v;
  };
  const end = () => node._flags & REACTIVE_DISPOSED && land(flight);
  return accessor(
    computed(prev => {
      node = getOwner();
      node._config |= CONFIG_REDERIVE;
      cleanup(end);
      // Answered: the node has a real committed answer — not a first load,
      // not a `loadingValue` window still open (both are loading, F-9).
      // Sticky, and read before this wrapper ever opens the window itself.
      const open = (answered ||= !(node._statusFlags & STATUS_UNINITIALIZED) && !node._loading);
      if (open) ((node._loading = true), (node._config |= CONFIG_LAGS));
      try {
        const r = fn(prev);
        if (open && typeof r === "object" && r !== null) {
          if (isThenable(r)) {
            ask(r);
            const done = () => land(r, true);
            r.then(done, done);
            return serve(r);
          }
          const iterate = r[Symbol.asyncIterator];
          if (iterate) {
            // A stream's flight ends at its first yield (or its end).
            const it = iterate.call(r);
            let first = true;
            const done = () => land(s, true);
            const s = {
              // Registered ahead of the engine's own reaction to the same
              // promise, as for a plain flight: verdict and value in one flush.
              next: v => {
                const p = it.next(v);
                if (first) {
                  first = false;
                  if (isThenable(p)) p.then(done, done);
                  else done();
                }
                return p;
              },
              return: v => it.return?.(v) ?? Promise.resolve({ done: true, value: v })
            };
            ask(s);
            return serve({ [Symbol.asyncIterator]: () => s });
          }
        }
        land(flight);
        return r;
      } catch (e) {
        // The wrapping form: an unready source parks the node in the window,
        // and the wait is its flight until the source's settle re-runs it.
        if (open && e instanceof NotReadyError) ask(e);
        else land(flight);
        throw e;
      }
    }, options)
  );
}

// `from` — a push source (callback subscription, Observable, AsyncIterable)
// as an AsyncIterable a computation can return or `for await` over:
//
//   const value = createMemo(() => from(source));
//
// A subscription keeps the latest value: values that arrive between pulls
// coalesce. An AsyncIterable streams through unchanged. Either way the source
// opens when the result is iterated and closes on return(), on supersede (the
// owning computation re-runs) and on owner disposal, whichever comes first.
function toSubscribe(source) {
  if (typeof source === "function") return source;
  // TC39 / RxJS interop: `Symbol.observable` (or "@@observable") returns the observable.
  // Read per call, not at module scope, so the module stays side-effect free.
  const $$observable = Symbol.observable;
  const interop = ($$observable && source?.[$$observable]) ?? source?.["@@observable"];
  const obs = typeof interop === "function" ? interop.call(source) : source;
  if (obs && typeof obs.subscribe === "function")
    return (next, error, complete) => obs.subscribe({ next, error, complete });
  return null;
}
const DONE = { value: undefined, done: true };
/**
 * Ties an iterator's `finish` to the computation iterating it. Solid reaches
 * an iterator's return() only when the compute returns the iterable directly.
 * Inside an async generator the generator's own return() is queued behind its
 * pending `await`, and owner cleanups wait for the replacing pass to commit,
 * so close on supersede, with owner disposal as the backstop. No owner (an
 * async compute iterating it after its first `await`, or plain iteration):
 * no registration and no diagnostic, so only return() closes it. Scopes that
 * forbid cleanups rely on supersede and return().
 */
function bindToOwner(finish) {
  const owner = getOwner();
  if (!owner) return;
  onSupersede(finish);
  if (!(owner._config & CONFIG_CHILDREN_FORBIDDEN)) cleanup(finish);
}
/**
 * An AsyncIterable as a stream bound to the iterating computation: every
 * value passes through (no coalescing). `finish` calls the inner return()
 * once and settles pending pulls as done, so a `for await` over it exits as
 * soon as the computation re-runs — even when the inner iterator is an async
 * generator whose own return() the language queues behind its pending
 * `await`.
 */
function wrapIterable(source) {
  return {
    [Symbol.asyncIterator]() {
      const it = source[Symbol.asyncIterator]();
      let closed = false;
      const pending = new Set();
      const finish = () => {
        if (closed) return DONE;
        closed = true;
        for (const resolve of pending) resolve(DONE);
        pending.clear();
        try {
          const returned = it.return?.();
          if (isThenable(returned)) returned.then(undefined, () => {});
        } catch {}
        return DONE;
      };
      bindToOwner(finish);
      return {
        next(...args) {
          if (closed) return Promise.resolve(DONE);
          const step = it.next(...args);
          // A bare result (the producer's buffered fast path) passes through.
          if (!isThenable(step)) return step;
          return new Promise((resolve, reject) => {
            pending.add(resolve);
            step.then(
              r => pending.delete(resolve) && resolve(r),
              e => pending.delete(resolve) && reject(e)
            );
          });
        },
        return() {
          return Promise.resolve(finish());
        }
      };
    }
  };
}
const toCleanup = t =>
  typeof t === "function"
    ? t
    : t && typeof t.unsubscribe === "function"
      ? () => t.unsubscribe()
      : () => {};
/**
 * Converts a callback subscription, an Observable or an AsyncIterable into an
 * AsyncIterable for a Solid computation. A subscription keeps only the latest
 * value, and a value the source emits during `subscribe` is served without a
 * loading frame; an AsyncIterable streams every value through.
 *
 * The source closes on supersede — the owning computation re-running — and
 * on owner disposal. Inside an async generator memo that holds for a
 * `for await` that starts before the first `await`; one that starts after it
 * has no owner to bind to, so its source closes only when the loop ends.
 * Prefer returning `from(...)` from its own memo, with awaited inputs read
 * from upstream memos and transforms done in a downstream memo.
 *
 * Iterate the result with `for await` or return it from a computation: its
 * `next()` returns an already-buffered value as a bare result, not a promise.
 *
 * @example
 * ```ts
 * const position = createMemo(() =>
 *   from<Position>((next, error) => {
 *     const id = navigator.geolocation.watchPosition(next, error);
 *     return () => navigator.geolocation.clearWatch(id);
 *   })
 * );
 * const ticks = createMemo(() => from(interval$(rate())));
 * ```
 */
function from(source) {
  const subscribe = toSubscribe(source);
  if (!subscribe) {
    if (source && typeof source[Symbol.asyncIterator] === "function") return wrapIterable(source);
    throw new TypeError("from(): expected a subscribe function, an Observable or an AsyncIterable");
  }
  return {
    [Symbol.asyncIterator]() {
      let latest;
      let has = false;
      let done = false;
      let failed = false;
      let failure;
      let waiting = null;
      let unsubscribe = null;
      let closed = false;
      const close = () => {
        if (closed) return;
        closed = true;
        done = true;
        const c = unsubscribe;
        unsubscribe = null;
        // A throwing unsubscribe must not escape: at supersede this runs
        // inside flush(), ahead of the replacing pass (the returned-iterator
        // path swallows it the same way).
        try {
          c?.();
        } catch {}
      };
      const settle = () => {
        if (!waiting) return;
        const w = waiting;
        if (has) {
          waiting = null;
          has = false;
          w.resolve({ value: latest, done: false });
        } else if (failed) {
          waiting = null;
          failed = false;
          w.reject(failure);
        } else if (done) {
          waiting = null;
          w.resolve({ value: undefined, done: true });
        }
      };
      const teardown = subscribe(
        value => {
          if (done) return;
          latest = value;
          has = true;
          settle();
        },
        err => {
          if (done) return;
          failed = true;
          failure = err;
          done = true;
          settle();
          close();
        },
        () => {
          if (done) return;
          done = true;
          settle();
          close();
        }
      );
      unsubscribe = toCleanup(teardown);
      // Completed or errored during subscribe: release what it just returned.
      if (closed) {
        const c = unsubscribe;
        unsubscribe = null;
        try {
          c();
        } catch {}
      }
      // Idempotent: return(), supersede and disposal may all reach it.
      const finish = () => {
        close();
        has = false;
        if (waiting) {
          const w = waiting;
          waiting = null;
          w.resolve(DONE);
        }
        return DONE;
      };
      bindToOwner(finish);
      return {
        next() {
          // A buffered value is returned as a bare result, not a promise: the
          // iterator consumer lands it in the same pass, so a source that
          // emits during subscribe (BehaviorSubject-style) shows its value
          // with no loading frame. `for await` accepts either form.
          if (has) {
            has = false;
            return { value: latest, done: false };
          }
          if (failed) {
            failed = false;
            return Promise.reject(failure);
          }
          if (done) return Promise.resolve({ value: undefined, done: true });
          return new Promise((resolve, reject) => {
            waiting = { resolve, reject };
          });
        },
        return() {
          return Promise.resolve(finish());
        }
      };
    }
  };
}

// CARVE 4: `flatten` lived in boundaries.ts but is the general children
// resolver the renderers (`web`, `universal`, `solid`'s `children()`) walk
// through — not a boundary feature. Moved verbatim so the boundary carve
// measures boundaries alone.
/**
 * Resolves nested accessors and arrays in a children tree to their rendered
 * values. See the `children()` helper in solid-js for the memoized form.
 */
function flatten(children, options) {
  if (typeof children === "function" && !children.length) {
    if (options?.doNotUnwrap) return children;
    do {
      children = children();
    } while (typeof children === "function" && !children.length);
  }
  if (
    options?.skipNonRendered &&
    (children == null || children === true || children === false || children === "")
  )
    return;
  if (Array.isArray(children)) {
    let results = [];
    if (flattenArray(children, results, options)) {
      return () => {
        let nested = [];
        flattenArray(results, nested, { ...options, doNotUnwrap: false });
        return nested;
      };
    }
    return results;
  }
  return children;
}
function flattenArray(children, results = [], options) {
  let notReady = null;
  let needsUnwrap = false;
  for (let i = 0; i < children.length; i++) {
    try {
      let child = children[i];
      if (typeof child === "function" && !child.length) {
        if (options?.doNotUnwrap) {
          results.push(child);
          needsUnwrap = true;
          continue;
        }
        do {
          child = child();
        } while (typeof child === "function" && !child.length);
      }
      if (Array.isArray(child)) {
        // OR, don't overwrite: an accessor already pushed under doNotUnwrap
        // still needs the resolving wrapper even when a later sibling
        // fragment contains no functions (#3133).
        needsUnwrap = flattenArray(child, results, options) || needsUnwrap;
      } else if (
        options?.skipNonRendered &&
        (child == null || child === true || child === false || child === "")
      ) {
        // skip
      } else results.push(child);
    } catch (e) {
      if (!(e instanceof NotReadyError)) throw e;
      notReady = e;
    }
  }
  if (notReady) throw notReady;
  return needsUnwrap;
}

/** Boundaries — the display consumers between a frame reader and the root:
 * `createLoadingBoundary` (`<Loading>`) and `createErrorBoundary`
 * (`<Errored>`).
 *
 * A boundary is an owner whose context names it (`BOUNDARY`), so every node
 * created under it finds it in O(1) and the enclosing boundaries through
 * `_parent`; a `tree` (the content, flattened) and an `output` that shows
 * the tree or the fallback. The output is a frame reader (CONFIG_REDERIVE):
 * status reaching it from its content re-derives it instead of marking it,
 * and its pass decides.
 *
 * Status: `GlobalQueue.notify` asks `catchStatus` before the root. The
 * nearest collecting boundary of that status records the reader and shows
 * its fallback, and the root never hears of it: a fallback-caught flight
 * holds nothing (F-3, F-5). A loading boundary showing content forwards.
 *
 * The hold asks `hiddenBy`: a reader behind any fallback is not on screen
 * and holds nothing (F-3).
 *
 * Reveal: the seam (`boundarySeam`) drops readers that settled, died, or
 * landed and committed; a fallback with none left re-derives its output. A
 * held reader is not settled: the content reveals at the commit (#3540).
 *
 * `on` (loading only): a pass of it after the mount arms the boundary for
 * the flush; anything pending under it is collected and the fallback shows
 * with the change's frame (D-17). */
/** Context key: the nearest boundary of a node, inherited at creation. */
const BOUNDARY = Symbol("boundary");
/** Context key: the reveal controller a Loading boundary created here is a
 * slot of (reveal.ts); a boundary clears it for its content — only direct
 * children are slots. */
const REVEAL = Symbol("reveal");
// Every call site checks the hooks themselves first (not only the slot or
// controller, which are never set without them): a build that does not
// import `createRevealOrder` folds the reveal-order arms away.
let revealHooks = null;
function setRevealHooks(h) {
  revealHooks = h;
}
/** Boundaries with readers to sweep (or an arm to resolve) at the seam. */
const collecting = new Set();
const boundaryOf = node => node._context[BOUNDARY];
/** An error boundary at or above `b` shows its fallback: content under it
 * is not on screen. */
const behindError = b =>
  !!b && ((b._type === STATUS_ERROR && b._fallback) || behindError(b._parent));
/** Catches what reaches it: an error boundary always; a loading one until it
 * has shown content, or while re-armed. Behind an error fallback nothing is
 * on screen, so the error boundary's recovery re-arms the subtree: a loading
 * boundary there has not shown content (D-41, F-4, #4627) — a query; the
 * boundary is fresh again once it catches (`caught`). */
const isCollecting = b =>
  b._type === STATUS_ERROR || !b._initialized || b._armed || behindError(b._parent);
/** The nearest loading boundary at or above `b` catches what is pending
 * (#4926). */
const freshAbove = b => !!b && (b._type & STATUS_PENDING ? isCollecting(b) : freshAbove(b._parent));
/** The statuses that keep reader `r` of `b`: `b`'s own and pending — and,
 * for a loading boundary behind an error fallback, the error of a reader
 * directly under it (an error boundary between them shows that error
 * itself; a held signal it waits on has no context): there is nothing to
 * reveal, the error shows above. Revealing re-derived the content, whose
 * re-read retried the error's source: pending again, caught again, without
 * end (#4870). The error's recovery answers the reader, which reveals it
 * with the recovered content. */
const keeps = (b, r) =>
  b._type |
  STATUS_PENDING |
  (behindError(b._parent) && r._context?.[BOUNDARY] === b ? STATUS_ERROR : 0);
/** A reader `b` waits on: recorded for its `on`, and `b` visited at the seam. */
function record(b, node, source = node) {
  b._readers.set(node, source);
  collecting.add(b);
}
/** The reader goes to the fallback: recorded, and the output re-derives
 * (a no-op while the output's own pass is running — its post-check
 * decides). */
function caught(b, node, error, source = node) {
  if (b._type === STATUS_ERROR) {
    // The error's source is the retry target, but the reader's selected
    // outcome owns publication. Do not lose a latest/optimistic lane when
    // converting its error into fallback content.
    // (Only the lane engine makes lanes: without it the clause folds away.)
    const lane = laneStageHook
      ? (passLane ?? (node._x?._transaction?._lane ? txOf(node) : null))
      : null;
    if (laneStageHook && !b._fallback) b._lane = lane;
    // An error boundary waits on what THREW (`StatusError.source`), not on
    // the reader that saw it, while the error is settled only when its
    // source is neither errored nor in flight and the reader is not pending
    // on the answer that replaces it (`prune`) — or when the
    // reader is disposed: a source no live reader holds holds nothing
    // (#4373). A source that died (a memo its reader owned, kept past the
    // pass that disposed it) settles nothing: the reader is waited on.
    // `reset` re-runs the source.
    error ??= node._x?._error;
    source = error?.source ?? node;
    if (source._flags & REACTIVE_DISPOSED) source = node;
    const caught = unwrapStatusError(error);
    // Subsequent errors update the existing fallback through this signal;
    // its message must publish with the swap, including after a first show.
    if (laneStageHook && lane) laneStageHook(b._error, lane, true, false);
    setSignal(b._error, caught);
    reportClientError(caught, b._owner, source);
  }
  // A loading boundary that catches has not shown content: fresh until it
  // does — already so unless it caught behind an error fallback, which the
  // recovery leaves catching (D-41, #4627); a re-arm's `flip` says it too.
  else b._initialized = false;
  record(b, node, source);
  // A loading boundary behind an error fallback that catches a pending
  // holds nothing above it: the error boundary's recovery shows its
  // fallback beside the recovered content (D-41, #4627).
  if (b._armed) flip(b);
  // Already showing the fallback: one more reader to wait on changes
  // nothing on screen. The fallback is instantiated once per show — a later
  // error reaches it through the `error` accessor, which is why it is one.
  if (!b._fallback) redraw(b);
  if (revealHooks && b._reveal !== null) revealHooks._changed(b._reveal);
}
/** A re-armed boundary found something pending: fresh again, and the swap
 * the arming pass asked for is that pass's lane's (if any): the output is
 * the lane's work from its next pass's head — not a re-pass of the
 * transaction holding its pending content, which would entangle that
 * transaction with the arming frame's (#3540 `on: () => latest(dep)`). */
function flip(b) {
  b._initialized = false;
  b._lane = b._armLane;
  const o = b._output;
  if (!b._lane && o._config & CONFIG_HELD && txOf(o) !== flushTransaction) unhold(o);
  if (laneStageHook && b._lane !== null && b._output !== null)
    laneStageHook(b._output, b._lane, true, false);
}
function redraw(b) {
  if (b._output !== null && !(b._output._flags & (REACTIVE_DISPOSED | REACTIVE_ZOMBIE))) {
    enqueueSub(b._output);
    schedule();
  }
}
/** `catchHook` (scheduler.ts): status from a frame reader, nearest boundary first.
 * A loading boundary on the way records a pending reader whether or not it
 * catches it (its `on` may collect it later). */
function catchStatus(node, flags, error) {
  if (flags === 0) {
    // A status cleared. Judged by the node's status now, against each
    // boundary's own rule (`prune`'s mask) — the error path clears pending
    // first and the node is still errored then. The last reader a fallback
    // waited on settling re-derives the output now, in the same heap (a
    // held reader's landing has joined its transaction, so the content
    // lands with its run, not a round after). Others still waiting: nothing
    // on screen changes, the fallback stays as it is. `node` here is an
    // effect reporting its own status. An error boundary records an effect
    // only while it is errored, and a staged error keeps STATUS_ERROR on a
    // node that had it (`stageError`), so `prune`'s CONFIG_STAGED_ERROR case
    // cannot reach this path (#4364). "Last" is judged as the seam would
    // (`prune`): readers the same held pass disposed — a list re-mapped
    // under its parked pass drops the rows that read the error — are gone,
    // and left for the seam, they revealed the stale content outside the
    // hold (#4526).
    if (collecting.size)
      for (let b = boundaryOf(node); b; b = b._parent)
        if (
          !(b._readers.get(node)?._statusFlags & keeps(b, node)) &&
          b._readers.delete(node) &&
          !prune(b, 0)
        ) {
          if (b._fallback) redraw(b);
          if (revealHooks && b._reveal !== null) revealHooks._changed(b._reveal);
        }
    return false;
  }
  for (let b = boundaryOf(node); b; b = b._parent) {
    if (!(b._type & flags)) {
      // A pending passing an error fallback by (D-38) is still a reader the
      // content needs: revealed beside its stale value, the frame tears (D-41,
      // F-1, #4592).
      if (b._fallback && b._type > flags) record(b, node);
      continue;
    }
    if (isCollecting(b)) {
      caught(b, node, error);
      return b._armed ? CAUGHT_REARMED : true;
    }
    record(b, node);
  }
  return false;
}
/** The nearest loading boundary that would catch `node` was re-armed this
 * flush. */
function armedOver(node) {
  for (let b = boundaryOf(node); b; b = b._parent)
    if (b._type & STATUS_PENDING && isCollecting(b)) return b._armed;
}
/** The nearest boundary on `r`'s chain showing its fallback, if any
 * (`hiddenHook`: `r` is behind a fallback). */
function hiddenBy(r, seen) {
  for (let b = boundaryOf(r); b; b = b._parent) if (b._fallback) return b;
  // An error boundary output passing a pending by (D-38) is on screen only
  // through the render effects it reaches (F-3). Reaching none, or only
  // effects behind fallbacks, it holds nothing (F-4, D-12, #4638); `h` is `1`
  // then. Each node is walked once (`seen`), so a memo lattice costs its size.
  let h,
    s = r._subs;
  if (seen || r._config & CONFIG_REDERIVE)
    for (
      seen ||= new Set(), h = r._type != EFFECT_RENDER;
      s && (h = seen.size == seen.add(s._sub).size || hiddenBy(s._sub, seen));
    )
      s = s._nextSub;
  return h;
}
/** `heldRunHook` (scheduler.ts): a queued run behind a fallback waits for the
 * reveal (F-3). A user effect would read a DOM that is not attached; a
 * render update mounts Portals and head tags outside the hidden subtree. A
 * render effect's first run is synchronous on creation and never comes
 * here. The reveal re-queues the run by type (`release`). */
function heldRun(node) {
  const b = hiddenBy(node);
  // A swap to the fallback still staged in a hold (an `on` re-arm the
  // frame holds) has not reached the screen: the content is still the
  // display, and a run there — a guess written over it — applies now
  // (D-17, F-1, #4987).
  // So is one whose content a guess lane carries beside it (#4999; lanes
  // only, `laneStageHook`).
  if (
    b === undefined ||
    b._output._config & (laneStageHook ? CONFIG_HELD | CONFIG_OVERRIDE : CONFIG_HELD)
  )
    return false;
  (b._runs ??= new Set()).add(node);
  return true;
}
/** The reveal: the runs held behind the fallback are this flush's, each in
 * its own phase — render updates with the frame that attaches the content,
 * user effects after it. */
function release(b) {
  if (b._runs === null) return;
  for (const n of b._runs) {
    const type = n._type;
    globalQueue.enqueue(
      type === EFFECT_TRACKED ? EFFECT_USER : type,
      type === EFFECT_TRACKED
        ? n._run
        : (n._boundRunEffect ??= GlobalQueue._runEffect.bind(null, n))
    );
  }
  b._runs = null;
}
/** `r`, held by error boundary `b` for the error `s` threw, is pending
 * again after `s` answered, and a loading boundary between them that has
 * not shown content catches it: that boundary is its display, beside the
 * recovered content (D-41, F-4, #4627). Not while that boundary's swap to
 * its fallback is staged in a hold (a recovery written in an action's tick):
 * the error stays up until the hold reveals both (D-41, F-1). */
function caughtBelow(b, r, s) {
  if (!(s._statusFlags & (STATUS_ERROR | STATUS_PENDING) || r._statusFlags & STATUS_ERROR))
    for (let l = boundaryOf(r); l && l !== b; l = l._parent)
      if (l._type & STATUS_PENDING && isCollecting(l)) return !(l._output._config & CONFIG_HELD);
  return false;
}
/** `n` reads `s`, directly or through derivations wherever they live (a
 * hoisted memo, a derived store's derive, a loading boundary's output). Each
 * upstream node is walked once (`seen`), so a shared upstream graph costs its
 * size, not its paths. */
function reads(n, s, seen) {
  for (let l = n._deps, h; l; l = l._nextDep)
    if ((h = l._dep) === s || ((seen ||= new Set()).size < seen.add(h).size && reads(h, s, seen)))
      return 1;
}
/** Drop the readers that settled or died. Settled: a loading reader no
 * longer pending; an error's source neither errored nor in flight, or no
 * longer read by the reader that caught it (D-41, #4644). A held reader is
 * not settled: the content reveals at the commit. A reader pending again
 * after the source that threw answered has no new answer yet: the error
 * stays the display (D-41), unless a loading boundary under the error one
 * catches it (`caughtBelow`, #4627).
 *
 * `pass` 0: a reader clearing asks whether it was the last (`catchStatus`).
 * The first kept reader ends the walk, so a recovery of N readers walks
 * each once (#4526). */
function prune(b, pass) {
  const mask = b._type | STATUS_PENDING;
  for (const [r, s] of b._readers) {
    if (!((r._flags | s._flags) & REACTIVE_DISPOSED)) {
      const k = keeps(b, r);
      // An error staged on its source is not settled: it publishes with the
      // commit (D-41, #4364). Nor a verdict reader queued to re-derive: its pass,
      // not its source's settling, is its answer (F-9, #4568).
      if (
        ((r._statusFlags & k || (s._statusFlags & k && reads(r, s))) &&
          !(mask & STATUS_ERROR && caughtBelow(b, r, s))) ||
        (mask & STATUS_ERROR && s._config & CONFIG_STAGED_ERROR) ||
        (r._config & CONFIG_VERDICT && r._flags & REACTIVE_IN_HEAP)
      ) {
        if (pass === 0) break;
        continue;
      }
      if (
        (s !== b._tree || b._armed) &&
        (s._config & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD
      ) {
        // Landed, held: the content is that transaction's — the output's
        // pass enters it and the reveal lands with the reader's run. The
        // seam, past its park decision, waits for the commit instead. (A
        // lane's reader reveals with the lane, this round.) A boundary that
        // has not shown content catches it instead: the fallback stays until
        // the reader is committed (#3540: what a fresh boundary catches holds
        // nothing outside it). Not an error boundary (born `_initialized`):
        // what it caught in a hold was that hold's staged error (D-41), and an
        // answer landing in the same hold replaces it there — the content
        // reveals with the commit, and nothing ever shows the superseded
        // error (D-41, #4527).
        if (!pass || !b._initialized) continue;
        joinFuture(txOf(s));
      }
    }
    b._readers.delete(r);
  }
  return b._readers.size;
}
/** Reveal order: a slot is ready when its content can show — it has shown
 * content, or nothing under it is unready (the tree neither pending nor
 * born held, no reader waited on). */
function ready(b) {
  return (
    b._initialized ||
    (!(b._tree._statusFlags & STATUS_PENDING) &&
      !(
        b._tree._config & CONFIG_HELD &&
        (b._output === null || b._output._statusFlags & STATUS_UNINITIALIZED)
      ) &&
      prune(b) === 0)
  );
}
/** `boundarySeamHook` (scheduler.ts), end of the seam: readers that settled, died,
 * or landed and committed are dropped; a fallback with none left reveals
 * next round. An arm resolves: nothing collected, nothing happened. */
function boundarySeam() {
  for (const b of collecting) {
    const flags = b._output._flags;
    if (flags & REACTIVE_DISPOSED) {
      collecting.delete(b);
      continue;
    }
    // A parked frame's boundary (zombie) keeps its state for its revival.
    if (flags & REACTIVE_ZOMBIE) continue;
    // A fresh boundary's content that derives from a write a hold keeps
    // reveals with that hold's commit (F-7, D-12): the boundary waits on
    // the held node, kept like a held reader until it commits (`prune`).
    // The node a settled reader reads: an answer staged there — a staged
    // error marks no frame reader (D-41) — left the reader settled on its
    // stale committed value (#4591). The node a first load it waits on
    // reads: the seam keeps a loading source out of every hold (#2937,
    // D-6), so its answer landed mainline, torn from the frame a write's
    // hold keeps on screen (F-1, #4593); under an open action it still
    // reveals at its landing (#2937). Not a held reader: it reveals with its
    // own commit.
    if (!b._initialized)
      for (const [r] of b._readers)
        for (const n of r._statusFlags & STATUS_PENDING ? r._x?._pendingSources || [] : [r])
          if (!(n._config & CONFIG_HELD))
            for (let d = n._deps; d; d = d._nextDep) {
              const h = d._dep;
              if (h._config & CONFIG_HELD && (n === r || !txOf(h)._open)) record(b, h);
            }
    prune(b);
    const o = b._output;
    // A fallback staged into the very hold its readers landed in folds back:
    // the content is ready when that hold commits (D-17, #4379). A loading
    // fallback only: an error boundary's is that hold's staged error (D-41),
    // and folding back re-derived the output into it every round (#4480).
    if (
      b._type & STATUS_PENDING &&
      !b._initialized &&
      o._config & CONFIG_HELD &&
      [...b._readers].every(
        ([, s]) =>
          !(s._statusFlags & STATUS_PENDING) && s._config & CONFIG_HELD && txOf(s) === txOf(o)
      )
    ) {
      b._readers.clear();
      b._initialized = true;
    }
    if (b._armed && !b._initialized && !b._ahead) reportOutsideHold(b);
    b._armed = false;
    if (b._readers.size === 0) {
      collecting.delete(b);
      if (b._fallback) redraw(b);
      if (revealHooks && b._reveal !== null) revealHooks._changed(b._reveal);
    }
  }
}
/** DEV, at the re-arm that flipped a boundary to its fallback: a source it
 * now waits on is also read by a frame reader outside it — the frame waits
 * on the very source, and the fallback can never be seen. Structural, so
 * reported once, at the change, naming the source; a display-ahead arm
 * (`latest()` in `on`) is the user's choice and not reported. */
function reportOutsideHold(b) {
  // A frame reader of the source outside `b`: a render effect, or another
  // boundary's tree (its output forwards the pending to the frame) — reading
  // it directly or through one pending memo (a boundary's content memo).
  const outside = (reader, hop) => {
    if (!(reader._statusFlags & STATUS_PENDING) || reader._flags & REACTIVE_DISPOSED) return false;
    if (reader._type !== EFFECT_RENDER && boundaryOf(reader)?._tree !== reader) {
      if (hop)
        for (let s = reader._subs; s !== null; s = s._nextSub)
          if (outside(s._sub, false)) return true;
      return false;
    }
    // Behind a fallback, or under a boundary whose output only such content
    // reads — an error boundary built outside `b`: the fallback is seen
    // (D-17, #4639).
    const rb = boundaryOf(reader);
    if (hiddenBy(reader) || (rb?._tree === reader && rb._output && hiddenBy(rb._output)))
      return false;
    for (let o = boundaryOf(reader); o; o = o._parent) if (o === b) return false;
    return true;
  };
  for (const r of b._readers.keys()) {
    const sources = r._x?._pendingSources;
    if (sources === undefined) continue;
    for (const source of sources) {
      // Only the flight the change itself started (D-17): an outside reader
      // of an earlier write's flight holds nothing the change waits on.
      if (source._time !== clock) continue;
      for (let s = source._subs; s !== null; s = s._nextSub) {
        if (!outside(s._sub, true)) continue;
        const name = source._name;
        reportDiagnostic(
          emitDiagnostic(
            {
              code: "LOADING_ON_OUTSIDE_HOLD",
              kind: "async",
              severity: "warn",
              message:
                "[LOADING_ON_OUTSIDE_HOLD] `on` re-armed a Loading boundary, but " +
                `${name ? `\`${name}\`` : "a source it is waiting on"} is also read outside it and holds the frame: the fallback can never be seen — the frame waits on the very source the boundary is waiting on. ` +
                "Move the outside read under the boundary so one hold owns the data. (Reading `latest()` in `on` shows the fallback now, beside the held frame.)",
              nodeName: name,
              data: { source: name }
            },
            b._owner
          )
        );
        return;
      }
    }
  }
}
/** `on` changed after the mount: fresh again if anything under it is
 * pending now; collecting for the rest of the flush either way. */
function arm(b) {
  if (!b._initialized) {
    // Already re-armed and on its fallback, as the frame's work — and the
    // `on` pass arms again as lane work: its verdict was provisional (a
    // memo of `isPending(m2)` is a watcher until the flush parks, then the
    // holder's verdict lane's — verdict.ts), and the swap it asked for is
    // display-ahead after all (#3528). Re-home the swap: the output re-runs
    // as the lane's, shown now beside the held frame.
    if (b._fallback && b._lane === null && b._armLane === null && passLane !== null) {
      b._armLane = passLane;
      b._ahead = true;
      // The swap the frame's pass staged (held with the frame) is void: the
      // lane's pass stages it anew (`flip` lists the output in the lane,
      // which voids its staging), as a watcher's provisional value is
      // voided at the seam (verdict.ts `verdictSeam`).
      flip(b);
      redraw(b);
    }
    return;
  }
  b._armed = true;
  // The swap is the change's work: the output joins it (D-12, #3540).
  b._output._config &= ~CONFIG_FRAME;
  // A verdict read in `on` (`isPending`, or a memo of one — #3528's
  // `createMemo(() => isPending(m2))`: the mark carries, core.ts `read`)
  // whose frame has no transaction yet: the swap is still display-ahead — the frame gets one now (it lands
  // at this very seam if nothing holds it), and the swap is its verdict
  // lane's work, shown ahead of the frame's own runs (#3528).
  let lane = passLane;
  if (lane === null && globalQueue._running && getOwner()._config & CONFIG_VERDICT) {
    joinFuture(null);
    lane = GlobalQueue._verdictLane(flushTransaction);
  }
  b._armLane = lane;
  b._ahead = lane !== null;
  collecting.add(b);
  const n = b._readers.size;
  heldUnder(b._owner, flushTransaction, r => record(b, r));
  for (const [r] of b._readers) {
    if (
      b._readers.size !== n ||
      (r._statusFlags & STATUS_PENDING && !(r._flags & REACTIVE_DISPOSED))
    ) {
      // What it collects was forwarded to the boundaries above too; it is this
      // boundary's alone now, the nearest catcher: an outer one re-armed by the
      // same tick does not wait for it (D-12).
      for (let a = b._parent; a !== null; a = a._parent)
        // An outer error boundary keys its readers too, but waits on their
        // sources: a re-armed Loading taking the pending does not settle it.
        if (a._type & STATUS_PENDING) for (const [o] of b._readers) a._readers.delete(o);
      flip(b);
      redraw(b);
      return;
    }
  }
}
/** The error boundary's `reset`: re-run what threw (a comparator throw
 * names a signal — nothing to re-run). */
function reset(b) {
  // A snapshot: a source failing again is caught — and re-added — mid-walk.
  for (const r of new Set(b._readers.values()))
    if (r._fn !== undefined && !(r._flags & REACTIVE_DISPOSED)) recompute(r);
  schedule();
}
/**
 * Dev CHECK: the `on` dependencies evaluated to a function — `<Loading
 * on={route}>` or `on={() => route()}` passes the accessor itself, a stable
 * value that is never called, so nothing is read and the boundary never
 * re-arms. Checked on the first run only: the shape is static.
 */
function reportOnAccessor(b) {
  reportDiagnostic(
    emitDiagnostic(
      {
        code: "LOADING_ON_ACCESSOR",
        kind: "async",
        severity: "warn",
        message:
          "[LOADING_ON_ACCESSOR] A Loading boundary's `on` received a function. `on` takes the " +
          "dependency's value — `on={route()}`, or `on={[query(), page()]}` for several — and an " +
          "accessor passed instead is never called, so nothing is read and the boundary never re-arms."
      },
      b._owner
    )
  );
}
function createBoundary(type, fn, show, onFn) {
  if (!getOwner()) {
    const message =
      "[NO_OWNER_BOUNDARY] Boundaries created outside a reactive context will never be disposed.";
    reportDiagnostic(
      emitDiagnostic({
        code: "NO_OWNER_BOUNDARY",
        kind: "lifecycle",
        severity: "warn",
        message,
        data: { boundaryType: type === STATUS_PENDING ? "loading" : "error" }
      })
    );
  }
  const owner = createOwner();
  const b = {
    _type: type,
    _parent: boundaryOf(owner) ?? null,
    _owner: owner,
    _tree: null,
    _output: null,
    _readers: new Map(),
    // Loading only: an error boundary has no first-content wait (`prune`).
    _initialized: type === STATUS_ERROR,
    _armed: false,
    _armLane: null,
    _lane: null,
    _fallback: false,
    _runs: null,
    _reveal: null,
    _gated: false,
    _collapsed: false,
    _error: type === STATUS_ERROR ? signal(undefined, ERROR_SIGNAL) : null,
    _show: show,
    _fb: null
  };
  const controller = type === STATUS_PENDING ? owner._context[REVEAL] : null;
  if (getOwner())
    cleanup(() => {
      collecting.delete(b);
      if (revealHooks && controller) revealHooks._unregister(controller, b);
    });
  // The `on` dependencies live OUTSIDE the boundary, as the condition of a
  // `<Show>` wrapping it would: created before the owner's context names it.
  // A source of it going pending re-runs it (CONFIG_REDERIVE) rather than
  // marking it: the pass catches the NotReady — it is a notification, it
  // does not suspend the parent — and re-arms.
  if (onFn) {
    let mounted = false;
    runWithOwner(owner, () =>
      computed(() => {
        try {
          const deps = onFn();
          if (true && !mounted && typeof deps === "function") reportOnAccessor(b);
        } catch (e) {
          // (D-42, dev: this catch is the re-arm's mechanism, not a user's.)
          if (true) forgetThrownRead();
          if (!(e instanceof NotReadyError)) {
            if (!globalQueue.notify(getOwner(), STATUS_ERROR, STATUS_ERROR, e)) {
              if (true) haltReactivityAt(e, getOwner());
              throw e;
            }
            return;
          }
        }
        if (mounted) arm(b);
        else mounted = true;
      })
    )._config |= CONFIG_REDERIVE;
  }
  const context = { ...owner._context, [BOUNDARY]: b };
  if (revealHooks !== null) context[REVEAL] = null;
  owner._context = context;
  const tree = runWithOwner(owner, () => {
    const c = true ? computed(fn, { name: "children" }) : computed(fn);
    return true
      ? computed(() => flatten(read(c)), { name: "boundary" })
      : computed(() => flatten(read(c)));
  });
  b._tree = tree;
  // A slot of the reveal order in its context (reveal.ts): gated until the
  // controller releases it — before the output's first pass.
  if (revealHooks && controller) revealHooks._register(controller, b);
  const output = computed(
    () => {
      // Structure, not a derivation of its content (CONFIG_FRAME, #4283): it
      // reads a hold it is not part of as the screen, as the render effect
      // flattening the same children without a boundary would. Not while a
      // re-arm's swap is the change's work (`arm`), until that swap commits
      // (D-12, #3540).
      const self = getOwner();
      // Reading the retained tree need not read the leaf that recovered.
      // Keep the ahead swap's lane for recovery as well as for the fallback;
      // otherwise that successful answer is parked in the input's hold.
      if (laneStageHook && b._type === STATUS_ERROR) {
        if (b._fallback && self._config & CONFIG_OVERRIDE) b._lane ??= txOf(self);
        if (b._lane !== null) {
          setPassLane(b._lane);
          self._flags |= REACTIVE_LANE_READ;
          b._lane = null;
        }
      }
      if (!b._armed && !(self._config & CONFIG_HELD)) self._config |= CONFIG_FRAME;
      // Reveal order (reveal.ts): a gated slot shows its fallback — or, in a
      // collapsed tail, nothing — whatever its content's state; the
      // controller is told the pass ran (its readiness may have changed) and
      // releases it in turn.
      if (b._gated) {
        // Linked without being read: the tree settling (or changing)
        // re-derives this pass, which is how the controller hears of it.
        // It may release this very slot (the frontier whose content just
        // settled): the pass goes on to show it — the redraw it asks for is
        // refused mid-pass and not needed.
        link(tree, self);
        if (revealHooks && b._reveal !== null) revealHooks._changed(b._reveal);
        if (b._gated) {
          if (b._collapsed) {
            b._fallback = true;
            return undefined;
          }
          return fallback(b);
        }
      }
      // A tree carrying a status is not read: an errored memo's re-read retries
      // it (F-9), and a pending one read as lane work would serve its committed
      // value (D-25) where the swap must still be the fallback. Linked, so its
      // recovery re-derives this pass. A swap to the fallback staged in a hold
      // leaves the content on screen: a lane pass derives it with its guesses
      // (D-17, D-33, F-1, #4999).
      const shown =
        laneStageHook &&
        !b._initialized &&
        self._config & CONFIG_HELD &&
        self._value !== b._fb?._value;
      const status = tree._statusFlags & (STATUS_PENDING | STATUS_ERROR);
      if (status !== 0) {
        link(tree, self);
        if (status & type) {
          if (isCollecting(b)) {
            caught(b, tree, undefined);
            return fallback(b);
          }
          // Forwarded: a loading boundary showing content keeps it and
          // holds the frame like any reader (recorded for its `on`).
          record(b, tree);
        }
        // An error boundary on its fallback is the display of everything
        // under it (F-3, F-12): a retry pending there (an error returned
        // above, so this status is a pending) keeps the fallback and
        // holds nothing outside it — the recovery re-derives this pass. Not
        // when the loading boundary above has not shown content (fresh, or
        // behind an error fallback, #4627): this fallback was never on
        // screen, and that boundary catches the pending (F-4, #4926).
        if (type === STATUS_ERROR && b._fallback && !freshAbove(b)) return fallback(b);
        // A pending passes an error boundary by (D-38); an error, a loading one; a
        // loading boundary showing content forwards its pending.
        throw tree._x._error;
      }
      if (isCollecting(b)) {
        // A boundary mounted over a held value shows its fallback now and the
        // content at the commit (F-7, #3540): entering the transaction would make
        // the output born held. One with a committed value reads a held tree and
        // enters: the landing reveals the content.
        if (
          b._readers.size !== 0 &&
          self._statusFlags & STATUS_UNINITIALIZED &&
          !(self._config & CONFIG_HELD)
        )
          return fallback(b);
        // Readers under it still unready: the fallback, the tree untouched.
        // The seam re-derives this pass when they settle.
        if (prune(b, true) !== 0 && !shown) return fallback(b);
      }
      let value;
      try {
        value = read(tree);
        // Returned content that is a store carries its family's error: the
        // probe reads it here, where the boundary catches it (F-12, D-41).
        // Left to `handleAsync`'s probe, it is the output's own error.
        if (b._error) untrack(() => isThenable(value));
      } catch (e) {
        if (e instanceof NotReadyError ? type === STATUS_PENDING : type === STATUS_ERROR) {
          if (isCollecting(b)) {
            // (D-42, dev: the boundary IS the catcher the throw is addressed to.)
            forgetThrownRead();
            caught(b, tree, e);
            return fallback(b);
          }
          // Forwarded: a loading boundary showing content keeps it and
          // holds the frame like any reader (recorded for its `on`).
          record(b, tree);
        }
        // A pending passes an error boundary by (D-38).
        throw e;
      }
      // Readers under it still unready: the fallback. So is a tree the read
      // itself ran into a pending: as lane work it is served the committed tree
      // (D-25), and the pass under it could not re-derive this one mid-pass
      // (#4856).
      if (isCollecting(b)) {
        if (tree._statusFlags & type & STATUS_PENDING) caught(b, tree);
        if (b._readers.size !== 0) return shown && passLane ? value : fallback(b);
      }
      b._initialized = true;
      // Observe: the fallback stops showing (a hide before its commit means
      // it never displayed — the engine knows the swap it was staged with).
      if (b._fallback && b._type & STATUS_PENDING && attrHooks !== null)
        attrHooks.boundaryFallback(b, b._tree, false);
      if (b._fallback && swapHook) swapHook();
      b._fallback = false;
      // The fallback dies when the content that replaces it is on screen —
      // with this flush's commits when the pass is the frame's, at the
      // landing when the frame parks (the run is held with it, as the old
      // children of a held pass are) — not now: a held swap keeps the
      // fallback on screen, live, until it commits. A pass back to the
      // fallback before then finds it still built and keeps it. A fallback
      // the screen does not show (a swap staged with a held frame, replaced
      // before it commits) dies now: what it reads holds nothing (S-22 row
      // 3′, #4268).
      const fb = b._fb;
      const drop = () => {
        if (!b._fallback && b._fb === fb) {
          // A fallback that threw (a rethrow the boundary does not
          // handle) is what an enclosing boundary waits on: it died, and
          // the seam of another round drops it there (#4270).
          if (fb._statusFlags & STATUS_ERROR) schedule();
          dispose(fb);
          b._fb = null;
        }
      };
      if (fb !== null)
        b._output._value !== fb._value ? drop() : globalQueue.enqueue(EFFECT_RENDER, drop);
      release(b);
      // Shown: the slot leaves its reveal order (a later re-arm gates nobody
      // else; the frontier moves on).
      if (revealHooks && b._reveal !== null) {
        const controller = b._reveal;
        b._reveal = null;
        revealHooks._changed(controller);
      }
      return value;
    },
    // Boundary structure, not a user source: its value is fallback-or-content
    // and legitimately swaps mid-hydration (resume), so it must never be
    // frozen by snapshot capture.
    { name: "value", _noSnapshot: true }
  );
  output._config |= CONFIG_REDERIVE;
  // The tree is structure too (the output's pass marks the output).
  tree._config |= CONFIG_FRAME;
  b._output = output;
  return accessor(output);
}
function fallback(b) {
  // (The output's pass.)
  const out = getOwner();
  // The swap a display-ahead re-arm asked for: this pass is the lane's — a
  // lane read by construction (REACTIVE_LANE_READ: `recompute`'s tail keeps
  // the pass in the lane).
  if (b._lane !== null) {
    setPassLane(b._lane);
    out._flags |= REACTIVE_LANE_READ;
    b._lane = null;
  }
  // Observe: a loading boundary starts showing its fallback. The swap is the
  // output's value; the engine watches it and counts the fallback displayed
  // when the screen shows it (a commit, a landing, a lane's reveal).
  if (!b._fallback && b._type & STATUS_PENDING && attrHooks !== null)
    attrHooks.boundaryFallback(b, b._tree, true, out);
  // The content goes behind the fallback: off screen (F-3), and so are its
  // head tags (`swapHook`, #4873).
  if (!b._fallback && swapHook) swapHook();
  b._fallback = true;
  // Built in its own scope, like `Show`'s function child, once per show
  // (D-41: the fallback is the display until a new answer replaces the
  // error): a re-derive of the output while it shows — a second reader
  // caught, the same error propagated again through a sibling's re-run, a
  // retry pending after `reset()`, a later error — keeps the built
  // fallback, its DOM and focus with it (#3991). A later error reaches it
  // through the error accessor; only a fallback reading the accessor at its
  // top level is rebuilt by it. Created beside the output — in its owner,
  // outside the boundary, as the output is: its errors and runs are the
  // enclosing boundary's — with the output's id, so what it builds gets the
  // ids a build under the output would; disposed once content is on screen.
  // Lazy (`1`: a byte under `true`), so its first pass runs in the output's
  // read below: the pass that creates it is the output's, and born held asks
  // the output — what it read — not the owner the fallback lives in (D-25;
  // #4428: an enclosing pass that joined a hold, while the output read nothing
  // held). (Lazy also releases it when its one reader, the output, stops
  // reading it: where `drop` below disposes it, or with the commit of a held
  // swap.)
  if (b._fb === null)
    b._fb = runWithOwner(out._parent, () => computed(() => b._show(b), { id: out.id, lazy: 1 }));
  // The swap is structure (#3540): what the fallback reads ahead — a
  // verdict (`latest()`), a lane — is the fallback's display-ahead, not the
  // swap's: the output's pass stays where it was (F-6 stays inside the
  // fallback, #4337).
  const p = passLane;
  const f = out._flags;
  const v = read(b._fb);
  setPassLane(p);
  out._flags ^= (out._flags ^ f) & (REACTIVE_PROBED | REACTIVE_LANE_READ);
  return v;
}
const ERROR_SIGNAL = { ownedWrite: true, _noSnapshot: true };
/**
 * Lower-level primitive that backs the `<Loading>` flow control. Catches
 * pending async reads inside `fn` and renders `fallback` until they settle.
 *
 * App code should use `<Loading fallback={...}>` instead — reach for this only
 * when authoring custom boundary components.
 *
 * @param fn the tracked subtree
 * @param fallback the fallback shown while async reads in `fn` are unresolved
 * @param options `on` — a dependency list: a tracked function whose reads
 *   re-arm the boundary. Its return value is irrelevant (never compared);
 *   what matters is what it reads. Without `on`, a boundary that has shown
 *   content keeps it through a refetch (the pending holds with the
 *   transaction). With `on`, a write to anything it reads makes the boundary
 *   fresh again: it stops waiting on its current content, and if something
 *   under it is pending it shows `fallback` until the new content is ready;
 *   if nothing is pending, the notification is a no-op. The fallback lands
 *   with the same frame as the change that caused it — now, when nothing
 *   else holds that frame; together with the rest of the new page during a
 *   held navigation, not before it. If the same data is also read outside
 *   the boundary, the frame waits on it and the fallback can never be seen
 *   (DEV warns `LOADING_ON_OUTSIDE_HOLD`); the fix is structural — move the
 *   outside read under the boundary so one hold owns the data. A frame held
 *   past the content's landing by something else (the write's action, other
 *   pending data) also shows no fallback; that is a race the fallback may
 *   lose, a legitimate outcome, and not reported. A display-ahead read in
 *   `on` (`latest()`) shows the fallback now, beside the held frame; that
 *   is a capability, not the recommended shape. Optimistic writes and a
 *   source going pending notify like any other. The children are not
 *   re-created — they stay alive behind the fallback.
 *
 * @example
 * ```tsx
 * // Custom boundary component built on top of the primitive.
 * function MyLoading(props: { fallback: JSX.Element; children: JSX.Element }) {
 *   return createLoadingBoundary(
 *     () => props.children,
 *     () => props.fallback
 *   ) as unknown as JSX.Element;
 * }
 * ```
 */
function createLoadingBoundary(fn, fallback, options) {
  return createBoundary(STATUS_PENDING, fn, () => fallback(), options?.on);
}
/**
 * Lower-level primitive that backs the `<Errored>` flow control. Catches
 * thrown errors inside `fn` and invokes `fallback(error, reset)` instead.
 * `error` is an accessor for the latest captured error; `reset()` recomputes
 * the failing sources so the boundary can attempt to recover.
 *
 * App code should use `<Errored fallback={...}>` instead — reach for this only
 * when authoring custom boundary components.
 *
 * @example
 * ```tsx
 * // Custom boundary that wraps the primitive and adds telemetry.
 * function TracedErrored(props: { fallback: (e: () => unknown) => JSX.Element; children: JSX.Element }) {
 *   return createErrorBoundary(
 *     () => props.children,
 *     (err, reset) => {
 *       reportError(err());
 *       return props.fallback(err);
 *     }
 *   ) as unknown as JSX.Element;
 * }
 * ```
 */
function createErrorBoundary(fn, fallback) {
  return createBoundary(STATUS_ERROR, fn, b => fallback(accessor(b._error), () => reset(b)));
}
// Installed at module evaluation — present exactly when something imports a
// boundary. An app without one pays the three null checks and nothing else.
installBoundaryHooks(
  catchStatus,
  (node, held) =>
    !held &&
    (node._statusFlags & STATUS_UNINITIALIZED
      ? catchStatus(node, STATUS_PENDING)
      : armedOver(node)),
  hiddenBy,
  boundarySeam,
  heldRun
);

/**
 * Reveal order — `createRevealOrder` (`<Reveal>`): the order in which a group
 * of Loading boundaries (its direct slots) may show their content.
 *
 * A controller owns each slot until the slot has shown content, and forces
 * on the slots it owns the state the order dictates: gated (the fallback,
 * whatever the content's state) and, in a collapsed sequential tail, nothing
 * at all. A nested reveal order is a composite slot of its parent — gated,
 * it gates all of its own; released, it runs its own order.
 *
 * - `sequential`: slots reveal front to back. The first unready slot is the
 *   frontier — its fallback shows; the slots behind it are gated (collapsed
 *   when `collapsed`); the ones before it have revealed.
 * - `together`: every owned slot is gated until all are minimally ready;
 *   then all are released at once.
 * - `natural`: each slot reveals on its own as it becomes ready.
 *
 * A slot graduates when it shows content: a later re-arm of it gates nobody
 * else, and slots appended after graduation form a fresh frontier. The
 * boundary tells the controller when its readiness changed (boundaries.ts:
 * caught, settled, shown, disposed); `order`/`collapsed` are tracked by a
 * computed of the group's own.
 */
const FALSE_ACCESSOR = () => false;
const SEQUENTIAL_ACCESSOR = () => "sequential";
const isController = slot => slot instanceof RevealController;
const slotReady = slot => (isController(slot) ? slot._isReady() : ready(slot));
const slotMinReady = slot => (isController(slot) ? slot._isMinReady() : ready(slot));
/** Force a slot's state. A boundary re-derives its output when it changes;
 * a nested controller passes the state on to its own slots. Released
 * (`!gated`), a nested controller leaves its parent's ownership and runs its
 * own order; a boundary leaves when it has shown content (boundaries.ts). */
function setSlotState(slot, controller, gated, collapsed) {
  if (isController(slot)) {
    slot._gated = gated;
    slot._collapsed = collapsed;
    slot._evaluate();
    // Released and ready — every slot of its own has shown — it graduates,
    // as a boundary does when it shows content. Released at the frontier
    // but not ready, it stays the parent's frontier.
    if (!gated && slot._ready && slot._parent === controller) slot._parent = null;
    return;
  }
  if (slot._gated !== gated || slot._collapsed !== collapsed) {
    slot._gated = gated;
    slot._collapsed = collapsed;
    redraw(slot);
  }
}
class RevealController {
  _order;
  _collapse;
  _slots = [];
  /** The controller gating this one, until it releases this one. */
  _parent = null;
  /** The state a parent forces on this whole group. */
  _gated = false;
  _collapsed = false;
  _ready = true;
  _minReady = true;
  _evaluating = false;
  constructor(_order, _collapse) {
    this._order = _order;
    this._collapse = _collapse;
  }
  /** A slot that has left this controller's ownership: a boundary that has
   * shown content, a nested group that is ready. Shown is shown — it counts
   * as ready whatever happens to it later (a re-arm gates nobody else). */
  _done(slot) {
    return (isController(slot) ? slot._parent : slot._reveal) !== this;
  }
  /** The slots this controller still owns, in registration order — the
   * ones the order's state is forced on. */
  _owned(fn) {
    for (let i = 0; i < this._slots.length; i++) {
      const slot = this._slots[i];
      if (!this._done(slot) && fn(slot) === false) return false;
    }
    return true;
  }
  /** Readiness is judged over every slot, done ones as ready. */
  _isReady() {
    for (let i = 0; i < this._slots.length; i++) {
      const slot = this._slots[i];
      if (!this._done(slot) && !slotReady(slot)) return false;
    }
    return true;
  }
  /** Something visible to show under its own order — what an enclosing
   * `together` waits for: `together` — every slot minimally ready;
   * `sequential` — the first slot; `natural` — any. */
  _isMinReady() {
    const order = untrack(this._order);
    const slots = this._slots;
    if (slots.length === 0) return true;
    if (order === "sequential") return this._done(slots[0]) || slotMinReady(slots[0]);
    for (let i = 0; i < slots.length; i++) {
      const min = this._done(slots[i]) || slotMinReady(slots[i]);
      if (order === "natural" ? min : !min) return order === "natural";
    }
    return order !== "natural";
  }
  _register(slot) {
    if (this._slots.includes(slot)) return;
    this._slots.push(slot);
    // Gated from the start (collapsed in a collapsed sequential tail); the
    // evaluation releases what the order allows.
    const collapsed = untrack(this._order) === "sequential" && !!untrack(this._collapse);
    if (isController(slot)) {
      slot._parent = this;
      slot._gated = true;
      slot._collapsed = collapsed;
      slot._evaluate();
    } else {
      slot._reveal = this;
      slot._gated = true;
      slot._collapsed = collapsed;
    }
    untrack(() => this._evaluate());
  }
  _unregister(slot) {
    const index = this._slots.indexOf(slot);
    if (index >= 0) this._slots.splice(index, 1);
    untrack(() => this._evaluate());
  }
  /** Apply the order to the owned slots — or, gated by a parent, its state
   * to all of them. */
  _evaluate() {
    if (this._evaluating) return;
    this._evaluating = true;
    const wasReady = this._ready;
    const wasMinReady = this._minReady;
    try {
      const order = untrack(this._order);
      const collapseTail = order === "sequential" && !!untrack(this._collapse);
      if (this._gated) {
        this._owned(slot => setSlotState(slot, this, true, this._collapsed));
      } else if (order === "natural") {
        this._owned(slot => {
          if (isController(slot)) setSlotState(slot, this, false, false);
          else setSlotState(slot, this, !ready(slot), false);
        });
      } else if (order === "together") {
        const minReady = this._owned(slotMinReady);
        this._owned(slot => setSlotState(slot, this, !minReady, false));
      } else {
        let pendingSeen = false;
        this._owned(slot => {
          if (pendingSeen) return setSlotState(slot, this, true, collapseTail);
          if (slotReady(slot)) return setSlotState(slot, this, false, false);
          pendingSeen = true;
          // The frontier: its fallback shows; a nested controller at the
          // frontier runs its own order.
          setSlotState(slot, this, isController(slot) ? false : true, false);
        });
      }
    } finally {
      this._ready = this._isReady();
      this._minReady = this._isMinReady();
      this._evaluating = false;
    }
    if (this._parent !== null && (wasReady !== this._ready || wasMinReady !== this._minReady))
      this._parent._evaluate();
  }
}
/**
 * Lower-level primitive behind `<Reveal>`: coordinates the order in which the
 * Loading boundaries created directly inside `fn` reveal their content.
 *
 * @param fn the subtree; the Loading boundaries it creates directly are the
 *   group's slots, a nested `createRevealOrder` a composite slot
 * @param options `order` — `"sequential"` (default: front to back, the
 *   first unready slot showing its fallback), `"together"` (all at once),
 *   `"natural"` (each on its own); `collapsed` — in a sequential group, the
 *   slots behind the frontier render nothing instead of their fallbacks
 */
function createRevealOrder(fn, options) {
  const owner = createOwner();
  const parent = owner._context[REVEAL];
  const controller = new RevealController(
    options?.order || SEQUENTIAL_ACCESSOR,
    options?.collapsed || FALSE_ACCESSOR
  );
  owner._context = { ...owner._context, [REVEAL]: controller };
  return runWithOwner(owner, () => {
    const value = fn();
    const evaluate = computed(() => {
      controller._order();
      controller._collapse();
      controller._evaluate();
    });
    if (true) evaluate._name = "reveal order";
    if (parent) {
      parent._register(controller);
      cleanup(() => parent._unregister(controller));
    }
    return value;
  });
}
// Installed at module evaluation — present exactly when something imports
// `createRevealOrder`; a Loading boundary without one pays a null check.
setRevealHooks({
  _register: (controller, b) => controller._register(b),
  _unregister: (controller, b) => controller._unregister(b),
  _changed: controller => controller._evaluate()
});

/**
 * Observe tier (diagnostics channel, attribution hook slot + interaction
 * frame): dev and observe builds. The attribution engine itself is the
 * `@solidjs/signals/attribution` entry.
 */
const OBSERVE = OBSERVE$1;
/** Dev tier (devtools hooks, graph traversal, console reporting): dev builds only. */
const DEV = DEV$1;

export {
  $PROXY,
  $RECORD,
  $REFRESH,
  $TARGET,
  $TRACK,
  ContextNotFoundError,
  DEV,
  MergeView,
  NoOwnerError,
  NotReadyError,
  OBSERVE,
  OmitView,
  ROOT_ERROR_HOOK,
  SOURCE_MEMO,
  SOURCE_MERGE,
  SOURCE_OMIT,
  SOURCE_PLAIN,
  SOURCE_PROXY,
  SUPPORTS_PROXY,
  TimeoutError,
  action,
  actionDepth,
  affects,
  clock,
  configureClientErrors,
  createContext,
  createDeferred,
  createEffect,
  createErrorBoundary,
  createLoadingBoundary,
  createMemo,
  createOptimistic,
  createOptimisticStore,
  createOwner,
  createProjection,
  createReaction,
  createRenderEffect,
  createRevealOrder,
  createRoot,
  createSignal,
  createStore,
  createTrackedEffect,
  deep,
  enableExternalSource,
  flatten,
  flush,
  forgetThrownRead,
  from,
  getContext,
  getObserver,
  getOwner,
  hasStaticKeys,
  isDisposed,
  isEqual,
  isPending,
  isStatic,
  isWrappable,
  latest,
  mapArray,
  markRaw,
  merge,
  mergeSources,
  mergeView,
  omit,
  omitView,
  onCleanup,
  onFlush,
  onSettled,
  reconcile,
  refresh,
  repeat,
  resolve,
  resolvedTable,
  runWithOwner,
  setContext,
  setReadWatch,
  setSignalIdentities,
  signalName,
  snapshot,
  sourceGet,
  sourceHas,
  sourceKeys,
  sourceOwners,
  storeHasFamily,
  storeHasOptimisticFamily,
  storeIsShallow,
  storeName,
  storePath,
  until,
  untrack,
  verdict,
  viewOf
};
