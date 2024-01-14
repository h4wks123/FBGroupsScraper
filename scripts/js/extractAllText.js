/**
 * Extracts all text content of a given xpath. A refineSelector can be used to further
 * refine the xpath selection using css selectors.
 *
 * @param {string} xpath - xpath to the node to track
 * @param {string} refineSelector - css selector to further refine the xpath selection
 * @returns {string} - all text content of the selected node
 */
function extractAllText(xpath, refineSelector = "") {
  let text = "";
  let content = document.evaluate(
    xpath,
    document,
    null,
    XPathResult.FIRST_ORDERED_NODE_TYPE,
    null,
  )?.singleNodeValue;

  if (refineSelector) {
    content = content.querySelector(refineSelector);
  }

  if (content) {
    const walker = document.createTreeWalker(content, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) text += walker.currentNode.textContent + "\n";
  }

  return text;
}
