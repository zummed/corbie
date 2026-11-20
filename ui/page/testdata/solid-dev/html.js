import { RawTextElements, trustedHTML, spread, claimElement, createComponent, insert, mergeProps, SVGElements, MathMLElements, VoidElements } from '@solidjs/web';

const OPEN_TAG_TOKEN = 0;
const CLOSE_TAG_TOKEN = 1;
const SLASH_TOKEN = 2;
const IDENTIFIER_TOKEN = 3;
const EQUALS_TOKEN = 4;
const STRING_TOKEN = 5;
const TEXT_TOKEN = 6;
const EXPRESSION_TOKEN = 7;
const SPREAD_TOKEN = 8;
const isIdentifierChar = code => {
  return isIdentifierStart(code) || code >= 48 && code <= 58 ||
  code === 46 ||
  code === 45
;
};
const isIdentifierStart = code => {
  return code >= 65 && code <= 90 ||
  code >= 97 && code <= 122 ||
  code === 95 ||
  code === 36
;
};
const isWhitespace = code => {
  return code >= 9 && code <= 13 || code === 32;
};
const STATE_TEXT = 0;
const STATE_TAG = 1;
const STATE_RAW_TEXT = 2;
const STATE_COMMENT = 3;
const STATE_LINE_COMMENT = 4;
const STATE_BLOCK_COMMENT = 5;
const STATE_BRACED_COMMENT = 6;
const tokenize = (strings, rawTextElements) => {
  const tokens = [];
  let state = STATE_TEXT;
  let lastTagName = "";
  let cursor = 0;
  for (let i = 0; i < strings.length; i++) {
    const str = strings[i];
    const len = str.length;
    cursor = 0;
    while (cursor < len) {
      switch (state) {
        case STATE_TEXT:
          {
            lastTagName = "";
            const nextTag = str.indexOf("<", cursor);
            const nextBracedComment = str.indexOf("{/*", cursor);
            const nextComment = nextBracedComment !== -1 && (nextTag === -1 || nextBracedComment < nextTag) ? nextBracedComment : -1;
            if (nextComment !== -1) {
              if (nextComment > cursor) tokens.push({
                type: TEXT_TOKEN,
                value: str.slice(cursor, nextComment)
              });
              state = STATE_BRACED_COMMENT;
              cursor = nextComment + 3;
            } else if (nextTag === -1) {
              if (cursor < len) tokens.push({
                type: TEXT_TOKEN,
                value: str.slice(cursor)
              });
              cursor = len;
            } else {
              if (nextTag > cursor) tokens.push({
                type: TEXT_TOKEN,
                value: str.slice(cursor, nextTag)
              });
              if (str[nextTag + 1] === "!" && str[nextTag + 2] === "-" && str[nextTag + 3] === "-") {
                state = STATE_COMMENT;
                cursor = nextTag + 4;
              } else {
                tokens.push({
                  type: OPEN_TAG_TOKEN
                });
                state = STATE_TAG;
                cursor = nextTag + 1;
              }
            }
            break;
          }
        case STATE_TAG:
          {
            const code = str.charCodeAt(cursor);
            if (isWhitespace(code)) {
              cursor++;
            } else if (code === 62) {
              if (rawTextElements.has(lastTagName) && tokens[tokens.length - 1]?.type !== SLASH_TOKEN && tokens[tokens.length - 2]?.type !== SLASH_TOKEN) {
                state = STATE_RAW_TEXT;
              } else {
                state = STATE_TEXT;
                lastTagName = "";
              }
              tokens.push({
                type: CLOSE_TAG_TOKEN
              });
              cursor++;
            } else if (code === 61) {
              tokens.push({
                type: EQUALS_TOKEN
              });
              cursor++;
            } else if (code === 47) {
              const next = str.charCodeAt(cursor + 1);
              const nextNonWhitespace = str.slice(cursor + 2).search(/\S/);
              const isShorthandClosingTag = next === 47 && tokens[tokens.length - 1]?.type === OPEN_TAG_TOKEN && nextNonWhitespace !== -1 && str[cursor + 2 + nextNonWhitespace] === ">";
              if (next === 47 && !isShorthandClosingTag) {
                state = STATE_LINE_COMMENT;
              } else if (next === 42) {
                state = STATE_BLOCK_COMMENT;
              } else {
                tokens.push({
                  type: SLASH_TOKEN
                });
                cursor++;
              }
            } else if (code === 34 || code === 39) {
              const char = str[cursor];
              const endQuoteIndex = str.indexOf(char, cursor + 1);
              if (endQuoteIndex === -1) {
                throw new Error(`Unterminated string at ${i}:${cursor}`);
              }
              tokens.push({
                type: STRING_TOKEN,
                value: str.slice(cursor + 1, endQuoteIndex),
                quote: char
              });
              cursor = endQuoteIndex + 1;
            } else if (isIdentifierStart(code)) {
              const start = cursor;
              while (cursor < len && isIdentifierChar(str.charCodeAt(cursor))) cursor++;
              const value = str.slice(start, cursor);
              if (tokens[tokens.length - 1]?.type === OPEN_TAG_TOKEN) {
                lastTagName = value;
              }
              tokens.push({
                type: IDENTIFIER_TOKEN,
                value
              });
            } else if (code === 46 && str[cursor + 1] === "." && str[cursor + 2] === ".") {
              tokens.push({
                type: SPREAD_TOKEN
              });
              cursor += 3;
            } else {
              throw new Error(`Unexpected Character: ${str[cursor]} at ${i}:${cursor}`);
            }
            break;
          }
        case STATE_RAW_TEXT:
          {
            const closeTagRegex = new RegExp(`<\\s*/\\s*${lastTagName}\\s*>`, "g");
            closeTagRegex.lastIndex = cursor;
            const match = closeTagRegex.exec(str);
            if (match) {
              const endOfRawIdx = match.index;
              if (endOfRawIdx > cursor) {
                tokens.push({
                  type: TEXT_TOKEN,
                  value: str.slice(cursor, endOfRawIdx)
                });
              }
              state = STATE_TEXT;
              cursor = endOfRawIdx;
              lastTagName = "";
            } else {
              tokens.push({
                type: TEXT_TOKEN,
                value: str.slice(cursor)
              });
              cursor = len;
            }
            break;
          }
        case STATE_COMMENT:
        case STATE_BRACED_COMMENT:
        case STATE_LINE_COMMENT:
        case STATE_BLOCK_COMMENT:
          {
            const commentEnd = state === STATE_LINE_COMMENT ? "\n" : state === STATE_BLOCK_COMMENT ? "*/" : state === STATE_BRACED_COMMENT ? "*/}" : "-->";
            const commentEndIndex = str.indexOf(commentEnd, cursor);
            if (commentEndIndex === -1) {
              cursor = len;
            } else {
              state = state === STATE_COMMENT || state === STATE_BRACED_COMMENT ? STATE_TEXT : STATE_TAG;
              cursor = commentEndIndex + commentEnd.length;
            }
            break;
          }
      }
    }
    if (i < strings.length - 1) {
      if (state === STATE_TEXT || state === STATE_TAG || state === STATE_RAW_TEXT) {
        tokens.push({
          type: EXPRESSION_TOKEN,
          value: i
        });
      }
    }
  }
  return tokens;
};

const isComponentNode = name => {
  const char = name.charCodeAt(0);
  return char >= 65 && char <= 90
;
};
const ROOT_NODE = 0;
const ELEMENT_NODE = 1;
const COMPONENT_NODE = 2;
const TEXT_NODE = 3;
const EXPRESSION_NODE = 4;
const BOOLEAN_PROP = 0;
const STATIC_PROP = 1;
const EXPRESSION_PROP = 2;
const SPREAD_PROP = 3;
const parse = (tokens, voidElements) => {
  const root = {
    type: ROOT_NODE,
    children: []
  };
  const stack = [root];
  let pos = 0;
  const len = tokens.length;
  while (pos < len) {
    const token = tokens[pos];
    const parent = stack[stack.length - 1];
    switch (token.type) {
      case TEXT_TOKEN:
        {
          const value = token.value;
          if (value.trim() === "") {
            const prevType = tokens[pos - 1]?.type;
            const nextType = tokens[pos + 1]?.type;
            if (prevType === CLOSE_TAG_TOKEN || nextType === OPEN_TAG_TOKEN) {
              pos++;
              continue;
            }
          }
          parent.children.push({
            type: TEXT_NODE,
            value
          });
          pos++;
          continue;
        }
      case EXPRESSION_TOKEN:
        {
          parent.children.push({
            type: EXPRESSION_NODE,
            value: token.value
          });
          pos++;
          continue;
        }
      case OPEN_TAG_TOKEN:
        {
          const nextToken = tokens[++pos];
          if (nextToken.type === SLASH_TOKEN) {
            const nameToken = tokens[++pos];
            const closeToken = tokens[++pos];
            const currentParent = stack[stack.length - 1];
            if (stack.length > 1 && closeToken.type === CLOSE_TAG_TOKEN && (nameToken?.type === IDENTIFIER_TOKEN && currentParent.name === nameToken.value || (nameToken?.type === EXPRESSION_TOKEN || nameToken.type === SLASH_TOKEN) && typeof currentParent.name === "number")) {
              const node = stack.pop();
              if (node?.type === ELEMENT_NODE && voidElements.has(node.name)) {
                node.children = [];
              }
              pos++;
              continue;
            }
            throw new Error(`Mismatched closing tag for <${currentParent.name}>`);
          }
          if (nextToken.type === IDENTIFIER_TOKEN || nextToken.type === EXPRESSION_TOKEN) {
            const tagName = nextToken.value;
            const node = {
              type: typeof tagName === "number" || isComponentNode(tagName) ? COMPONENT_NODE : ELEMENT_NODE,
              name: tagName,
              props: [],
              children: []
            };
            parent.children.push(node);
            pos++;
            while (pos < len) {
              const attrToken = tokens[pos];
              if (attrToken.type === CLOSE_TAG_TOKEN || attrToken.type === SLASH_TOKEN) {
                break;
              }
              if (attrToken.type === SPREAD_TOKEN) {
                const expr = tokens[pos + 1];
                if (expr?.type === EXPRESSION_TOKEN) {
                  node.props.push({
                    type: SPREAD_PROP,
                    value: expr.value
                  });
                  pos += 2;
                } else {
                  throw new Error(`Spread operator in <${node.name}> must be followed by an expression`);
                }
              } else if (attrToken.type === IDENTIFIER_TOKEN) {
                const name = attrToken.value;
                const next = tokens[pos + 1];
                if (next?.type === EQUALS_TOKEN) {
                  pos += 2;
                  const valToken = tokens[pos];
                  if (valToken.type === EXPRESSION_TOKEN) {
                    node.props.push({
                      name,
                      type: EXPRESSION_PROP,
                      value: valToken.value
                    });
                    pos++;
                  } else if (valToken.type === STRING_TOKEN) {
                    const quote = valToken.quote;
                    node.props.push({
                      name,
                      value: valToken.value,
                      quote,
                      type: STATIC_PROP
                    });
                    pos++;
                  } else {
                    throw new Error(`Attribute value for "${name}" in <${node.name}> must be an expression or a string`);
                  }
                } else {
                  node.props.push({
                    type: BOOLEAN_PROP,
                    name,
                    value: true
                  });
                  pos++;
                }
              } else {
                throw new Error(`Invalid attribute in <${node.name}>`);
              }
            }
            const endToken = tokens[pos];
            if (endToken.type === SLASH_TOKEN) {
              pos += 2;
            } else if (endToken.type === CLOSE_TAG_TOKEN) {
              pos++;
              stack.push(node);
            }
            continue;
          }
        }
      default:
        throw new Error(`Unexpected token: ${JSON.stringify(token)}  after <${stack[stack.length - 1].name}>`);
    }
  }
  if (stack.length > 1) {
    throw new Error(`Unclosed tag for <${stack[stack.length - 1].name}>`);
  }
  return root;
};

const flat = arr => {
  return arr.length === 1 ? arr[0] : arr;
};
function createHtml() {
  const cache = new WeakMap();
  const rawTextElements = new Set(RawTextElements);
  rawTextElements.delete("template");
  const walker = document.createTreeWalker(document, 129);
  const SVG_NS = "http://www.w3.org/2000/svg";
  const MATHML_NS = "http://www.w3.org/1998/Math/MathML";
  const createElement = (name, ns) => {
    ns = name === "svg" ? SVG_NS : name === "math" ? MATHML_NS : ns || (SVGElements.has(name) ? SVG_NS : MathMLElements.has(name) ? MATHML_NS : ns);
    return ns ? document.createElementNS(ns, name) : document.createElement(name);
  };
  const createTaggedJSX = components => {
    const tag = (strings, ...values) => {
      const root = getCachedRoot(strings);
      return renderChildren(root, values, components);
    };
    tag.components = components;
    tag.jsx = tag;
    tag.define = newComponents => {
      return createTaggedJSX({
        ...components,
        ...newComponents
      });
    };
    return tag;
  };
  const getCachedRoot = strings => {
    let root = cache.get(strings);
    if (!root) {
      root = parse(tokenize(strings, rawTextElements), VoidElements);
      buildTemplate(root, false);
      cache.set(strings, root);
    }
    return root;
  };
  const buildTemplate = (node, insideTemplate) => {
    if (node.type === ELEMENT_NODE) {
      if (!insideTemplate) {
        const template = document.createElement("template");
        template.content.appendChild(buildNodes(node));
        node.template = template;
        insideTemplate = true;
      }
      node.children.forEach(child => buildTemplate(child, insideTemplate));
    } else if (node.type === COMPONENT_NODE || node.type === ROOT_NODE) {
      if (node.type === COMPONENT_NODE) decodeStaticProps(node);
      node.children.forEach(child => buildTemplate(child, false));
    } else if (node.type === TEXT_NODE && !insideTemplate) {
      textTemplate.innerHTML = trustedHTML(node.value);
      node.value = textTemplate.content.textContent ?? "";
    }
  };
  const textTemplate = document.createElement("template");
  const decodeStaticProps = node => {
    for (const prop of node.props) {
      if (prop.type !== STATIC_PROP || !prop.value.includes("&")) continue;
      textTemplate.innerHTML = trustedHTML(`<i a="${prop.value.replace(/"/g, "&quot;")}"></i>`);
      prop.value = textTemplate.content.firstChild.getAttribute("a");
    }
  };
  const buildNodes = (node, ns) => {
    switch (node.type) {
      case TEXT_NODE:
        textTemplate.innerHTML = trustedHTML(node.value);
        return document.createTextNode(textTemplate.content.textContent ?? "");
      case EXPRESSION_NODE:
        return document.createComment("+");
      case COMPONENT_NODE:
        return document.createComment(node.name);
      case ELEMENT_NODE:
        let hasSpread = false;
        const elem = createElement(node.name, ns);
        const claimAttr = node.name === "a" ? "href" : node.name === "form" ? "action" : undefined;
        decodeStaticProps(node);
        node.props = node.props.filter(prop => {
          if (prop.type === STATIC_PROP) {
            if (prop.name.startsWith("prop:")) return true;
            elem.setAttribute(prop.name, prop.value);
            if (!hasSpread && prop.name === claimAttr) node.claim = true;
            return hasSpread;
          } else if (prop.type === BOOLEAN_PROP) {
            elem.setAttribute(prop.name, "");
            if (!hasSpread && prop.name === claimAttr) node.claim = true;
            return hasSpread;
          } else if (prop.type === SPREAD_PROP) {
            hasSpread = true;
            return hasSpread;
          }
          return true;
        });
        const childRoot = node.name === "template" ? elem.content : elem;
        const childNs = elem.namespaceURI === SVG_NS ? node.name === "foreignObject" ? undefined : SVG_NS : elem.namespaceURI === MATHML_NS ? MATHML_NS : undefined;
        childRoot.append(...node.children.map(child => buildNodes(child, childNs)));
        return elem;
    }
  };
  const renderNode = (node, values, components) => {
    switch (node.type) {
      case TEXT_NODE:
        return node.value;
      case EXPRESSION_NODE:
        return values[node.value];
      case COMPONENT_NODE:
        const component = typeof node.name === "string" ? components[node.name] : values[node.name];
        if (component && typeof component === "function") {
          return createComponent(component, gatherProps(node, values, components));
        } else {
          throw new Error(`Component "${node.name}" not found in registry`);
        }
      case ELEMENT_NODE:
        const element = renderChildren(node, values, components);
        const props = gatherProps(node, values, components);
        spread(element, props, true);
        if (node.claim) claimElement(element);
        return element;
    }
  };
  const renderChildren = (node, values, components) => {
    if (node.type !== ELEMENT_NODE || !node.template) {
      return flat(node.children.map(n => renderNode(n, values, components)));
    }
    const element = node.template.content.firstChild.cloneNode(true);
    walker.currentNode = element;
    const walkNodes = (nodes, walker) => {
      for (const node of nodes) {
        if (node.type === ELEMENT_NODE || node.type === EXPRESSION_NODE || node.type === COMPONENT_NODE) {
          const domNode = walker.nextNode();
          if (node.type === EXPRESSION_NODE || node.type === COMPONENT_NODE) {
            insert(domNode.parentNode, renderNode(node, values, components), domNode);
            walker.currentNode = domNode;
          } else {
            if (node.props.length) {
              const props = gatherProps(node, values, components);
              spread(domNode, props, true);
            }
            if (node.claim) claimElement(domNode);
            walkNodes(node.children, node.name === "template" ? document.createTreeWalker(domNode.content, 129) : walker);
          }
        }
      }
    };
    walkNodes(node.children, node.name === "template" ? document.createTreeWalker(element.content, 129) : walker);
    return element;
  };
  const gatherProps = (node, values, components, props = {}) => {
    const sources = [];
    let own = props;
    for (const prop of node.props) {
      switch (prop.type) {
        case BOOLEAN_PROP:
          own[prop.name] = true;
          break;
        case STATIC_PROP:
          own[prop.name] = prop.value;
          break;
        case EXPRESSION_PROP:
          applyGetter(own, prop.name, values[prop.value], node.type === COMPONENT_NODE);
          break;
        case SPREAD_PROP:
          const spreadValue = values[prop.value];
          if (!spreadValue || typeof spreadValue !== "object") throw new Error("Can only spread objects");
          sources.push(own, spreadValue);
          own = {};
          break;
      }
    }
    if (node.type === COMPONENT_NODE && node.children.length) {
      Object.defineProperty(own, "children", {
        get() {
          return renderChildren(node, values, components);
        }
      });
    }
    if (sources.length === 0) return own;
    sources.push(own);
    return mergeProps(...sources);
  };
  const applyGetter = (props, name, value, component) => {
    if (typeof value === "function" && value.length === 0 && (component || name !== "ref" && !/^on[A-Z]/.test(name))) {
      Object.defineProperty(props, name, {
        get() {
          return value();
        },
        enumerable: true
      });
    } else {
      props[name] = value;
    }
  };
  return createTaggedJSX({});
}
const html = createHtml();

export { html as default };
