const EMPTY = Object.freeze([]);

const noop = () => {};

const attribution = {
    enable: () => noop,
    disable: noop,
    history: () => EMPTY,
    markFlight: noop
};

// The named surface, inert: every fold empty, every query empty, every
// formatter blank. Typed against the real entry's exports so the two cannot
// drift.
const costs = () => ({
    scopes: [],
    writes: []
});

const feedback = () => ({
    sources: [],
    interactions: [],
    navigations: [],
    flights: [],
    fallbacks: []
});

const why = () => [];

const subscriptions = () => [];

const formatRerun = () => "";

const formatOrigin = () => "";

// Prod registers no roots: the graph has no observable size.
const graphSize = () => ({
    roots: 0,
    owners: 0,
    computations: 0,
    signals: 0,
    edges: 0
});

export { attribution, costs, feedback, formatOrigin, formatRerun, graphSize, subscriptions, why };