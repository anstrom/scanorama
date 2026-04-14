import "@testing-library/jest-dom/vitest";

// JSDOM does not implement scroll APIs used by TanStack Router's scroll
// restoration. Stub them out to suppress "Not implemented" console noise.
window.scrollTo = () => {};
