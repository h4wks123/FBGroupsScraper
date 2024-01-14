/**
 * Tracks whether a node is stable (i.e., has reached a state where changes are no longer expected).
 * The stability is tracked in the global variable object `window.isStable`, and can be
 * accessed by the key assigned to it.
 *
 * @param {string} xpath - xpath to the node to track
 * @param {string} key - key assigned to the node in the global variable object `window.isStable`
 * @param {number} debounce - debounce time in ms to consider that a node is stable
 */
function trackStability(xpath, key, debounce) {
  let node = document.evaluate(
    xpath,
    document,
    null,
    XPathResult.FIRST_ORDERED_NODE_TYPE,
    null,
  )?.singleNodeValue;

  if (node) {
    let timeout = null;
    if (!window.isStable) window.isStable = {}; // Ensures that the global variable object exists

    const observer = new MutationObserver(function () {
      window.isStable[key] = false;

      clearTimeout(timeout);
      timeout = setTimeout(() => (window.isStable[key] = true), debounce);
    });

    observer.observe(node, { childList: true });
  }
}
