(function(){function r(e,n,t){function o(i,f){if(!n[i]){if(!e[i]){var c="function"==typeof require&&require;if(!f&&c)return c(i,!0);if(u)return u(i,!0);var a=new Error("Cannot find module '"+i+"'");throw a.code="MODULE_NOT_FOUND",a}var p=n[i]={exports:{}};e[i][0].call(p.exports,function(r){var n=e[i][1][r];return o(n||r)},p,p.exports,r,e,n,t)}return n[i].exports}for(var u="function"==typeof require&&require,i=0;i<t.length;i++)o(t[i]);return o}return r})()({1:[function(require,module,exports){
'use strict'
var names = require('./names')
var morph = require('nanomorph')
var forms = require('submit-form-element')

var cache = {}
window.COHERENCE = {ids: cache}

var inflight = 0, timer, since, checking = false

//check wether user has this tab open, if it's not open
//don't check anything again until they come back.
//note: on my tiling window manager, it doesn't notice
//if you left this page open, but switch to another workspace.
//but if you switch tabs it does.

var onScreen = document.visibilityState == 'visible'
function setOnScreen () {
  if(onScreen) return
  onScreen = true
  check(since)
}
if(!document.visibilityState) {
  window.onfocus = setOnScreen
  window.onblur = function () {
    onScreen = false
  }
  window.onmouseover = setOnScreen
}
document.addEventListener('visibilitychange', function () {
  if(document.visibilityState === 'visible') setOnScreen()
  else onScreen = false
})

//-- util functions ---

function xhr (url, cb) {
  var req = new XMLHttpRequest()
  req.open('get', url)
  req.setRequestHeader('Content-Type', 'application/json')
  req.onload = function () {
    cb(null, req.response)
  }
  req.onerror = function () {
    cb(new Error(req.status + ':' + req.statusText))
  }
  req.send()
}

window.addEventListener('load', scan)

// --- forms ---

function isTag(element, type) {
  return element.tagName.toLowerCase() == type.toLowerCase()
}

var clicked_button = null, timer

//remember which button was clicked, or handle the click if it was a link.
window.addEventListener('click',function (ev) {
  //need this hack, because onsubmit event can't tell you what button was pressed.

  if(isTag(ev.target, 'button') || isTag(ev.target, 'input') && ev.target.type == 'submit') {
    clicked_button = ev.target
    clearTimeout(timer)
    timer = setTimeout(function () {
      clicked_button = null
    },0)
  }
  //if we have a target for a link click, apply that element.
  //unless ctrl is held down, which would open a new tab
  else if(!ev.ctrlKey && isTag(ev.target, 'a') && ev.target.dataset[names.Update]) {
    var update = document.getElementById(ev.target.dataset[names.Update])
    if(!update) return
    ev.preventDefault()

    //use getAttribute instead of ev.target.href because then it will just be / and not have
    //http:...com/...
    var href = (ev.target.dataset[names.OpenHref] || ev.target.getAttribute('href'))
    if(href)
      xhr('/' + names.Partial + href, function (err, content) {
        if(err) console.error(err) //TODO: what to do with error?
        else morph(update, content)
      })
  }
})

//handle form submit
window.addEventListener('submit', function (ev) {
  var form = ev.target
  if(form.dataset[names.Update] || form.dataset[names.Invalidate] || form.dataset[names.Reset]) {
    ev.preventDefault()
    forms.submit(form, clicked_button, function (err, content) {
      //what to do with error?
      if(form.dataset[names.Invalidate])
        update(form.dataset[names.Invalidate])
      if(form.dataset[names.Update]) {
        var target = document.getElementById(form.dataset[names.Update])
        morph(target, content)
      }
      if(form.dataset[names.Reset])
        form.reset()
    })
  }
})

// --- checking for and applying updates ------

function delay (errors) {
  return Math.max(Math.pow(2, errors || 0), 128) * 1e3
}

function schedule (delay) {
  clearTimeout(timer)
  delay = delay || 1e2
  timer = setTimeout(function () {
    if(!onScreen) return //don't check if the user isn't looking!
    console.log('check again', onScreen, document.visibilityState)
    check(since)
  }, delay/2 + delay*Math.random())
}

function scan () {
  if(since) throw new Error('only scan once!')
  since = Infinity
  ;[].forEach.call(
    document.querySelectorAll('[data-'+names.Timestamp+']'),
    function (el) {
      cache[el.dataset[names.Identity]] =
      since = isNaN(+el.dataset[names.Timestamp]) ? since : Math.min(since, +el.dataset[names.Timestamp])
    })

  //skip checking if there were no updatable elements found
  if(since != Infinity) check(since)
  else console.error('coherence: no updatable elements found')
}

// call the cache server, and see if there has been any updates
// since this page was rendered.
var errors = 0
function check (_since) {
  if(_since == undefined) throw new Error('undefined: since')
  if(checking) return
  checking = true
  xhr('/' + names.Coherence + '/' + names.Cache + '?since='+_since, function (err, data) {
    checking = false
    if(err) {
      errors ++
      console.error('error while checking cache, server maybe down?')
      console.error(err)
      return schedule(delay(errors))
    }
    var response, start, ids
    try { response = JSON.parse(data) } catch(_) {
      errors ++
      return schedule(delay(errors))
    }
    ids = response && response.ids
    start = response && response.start
    errors = 0
    if(ids && 'object' === typeof ids) {
      var ary = []
      for(var k in ids) {
        since = Math.max(since, ids[k] || 0)
        ary.push(k)
      }
    }
    since = Math.max(since, start || 0)

    for(var k in cache) {
      if(start > cache[k])
        ary.push(k)
    }

    if(Array.isArray(ary) && ary.length) ary.forEach(update)
    if(!inflight) schedule()
  })
}

function mutate (el, content) {
  // update an element with new content, using morphdom.

  // some node types cannot just simply be created anywhere.
  // (such as tbody, can only be inside a table)
  // if you just call morph(el, content) it becomes a flattened
  // string.
  // so, create the same node type as the parent.
  // (this will break if you try to update to a different node type)
  //
  // DocumentFragment looked promising here, but document
  // fragment does not have innerHTML! you can only
  // use it manually! (I guess I could send the html
  // encoded as json... but that wouldn't be as light weight)
  if(content) {
    var fakeParent = document.createElement(el.parentNode.tagName)
    fakeParent.innerHTML = content
    morph(el, fakeParent.firstChild)
    //sometimes, we want to send more than one tag.
    //so that the main tag is updated then some more are appended.
    //do this via a document-fragment, which means only
    //one reflow (faster layout).
    if(fakeParent.children.length > 1) {
      var df = document.createDocumentFragment()
      for(var i = 1; i< fakeParent.children.length; i++)
        df.appendChild(fakeParent.children[i])

      if(el.nextSibling)
        el.parentNode.insertBefore(df, el.nextSibling)
      else
        el.parentNode.appendChild(df)
    }
  } else {
    //if the replacement is empty, remove el.
    el.parentNode.removeChild(el)
  }
}

function update (id) {
  console.log('update:'+id)
  var el = document.querySelector('[data-'+names.Identity+'='+JSON.stringify(id)+']')
  if(!el) {
    console.log('could not find id:'+id)
    return
    //xxxxxxxx
  }
  //href to update this element
  var href = el.dataset[names.PartialHref]
  if(href) {
    inflight ++
    xhr('/'+names.Partial+href, function (err, content) {
      if(!err) mutate(el, content)
      //check again in one second
      if(--inflight) return
      schedule()
    })
  }
  else {
    console.error('cannot update element, missing data-'+names.PartialHref+' attribute')
  }
}


},{"./names":2,"nanomorph":4,"submit-form-element":10}],2:[function(require,module,exports){


module.exports = {
  //get /partial/..url to load url without layout.
  Partial: 'partial',
  Coherence: 'coherence', //url at which coherence specific things are under
  Script: 'browser', //name of the script which keeps ui up to date.
  Cache: 'cache', //query the cache state

  //the following are all set as data-* attributes

  Update: 'update',

  OpenHref: 'href',
  UpdateHref: 'href',
  PartialHref: 'href',

  Invalidate: 'invalidate',
  Reset: 'reset',
  Timestamp: 'ts',
  Identity: 'id'
}









},{}],3:[function(require,module,exports){
assert.notEqual = notEqual
assert.notOk = notOk
assert.equal = equal
assert.ok = assert

module.exports = assert

function equal (a, b, m) {
  assert(a == b, m) // eslint-disable-line eqeqeq
}

function notEqual (a, b, m) {
  assert(a != b, m) // eslint-disable-line eqeqeq
}

function notOk (t, m) {
  assert(!t, m)
}

function assert (t, m) {
  if (!t) throw new Error(m || 'AssertionError')
}

},{}],4:[function(require,module,exports){
var assert = require('nanoassert')
var morph = require('./lib/morph')

var TEXT_NODE = 3
// var DEBUG = false

module.exports = nanomorph

// Morph one tree into another tree
//
// no parent
//   -> same: diff and walk children
//   -> not same: replace and return
// old node doesn't exist
//   -> insert new node
// new node doesn't exist
//   -> delete old node
// nodes are not the same
//   -> diff nodes and apply patch to old node
// nodes are the same
//   -> walk all child nodes and append to old node
function nanomorph (oldTree, newTree, options) {
  // if (DEBUG) {
  //   console.log(
  //   'nanomorph\nold\n  %s\nnew\n  %s',
  //   oldTree && oldTree.outerHTML,
  //   newTree && newTree.outerHTML
  // )
  // }
  assert.equal(typeof oldTree, 'object', 'nanomorph: oldTree should be an object')
  assert.equal(typeof newTree, 'object', 'nanomorph: newTree should be an object')

  if (options && options.childrenOnly) {
    updateChildren(newTree, oldTree)
    return oldTree
  }

  assert.notEqual(
    newTree.nodeType,
    11,
    'nanomorph: newTree should have one root node (which is not a DocumentFragment)'
  )

  return walk(newTree, oldTree)
}

// Walk and morph a dom tree
function walk (newNode, oldNode) {
  // if (DEBUG) {
  //   console.log(
  //   'walk\nold\n  %s\nnew\n  %s',
  //   oldNode && oldNode.outerHTML,
  //   newNode && newNode.outerHTML
  // )
  // }
  if (!oldNode) {
    return newNode
  } else if (!newNode) {
    return null
  } else if (newNode.isSameNode && newNode.isSameNode(oldNode)) {
    return oldNode
  } else if (newNode.tagName !== oldNode.tagName || getComponentId(newNode) !== getComponentId(oldNode)) {
    return newNode
  } else {
    morph(newNode, oldNode)
    updateChildren(newNode, oldNode)
    return oldNode
  }
}

function getComponentId (node) {
  return node.dataset ? node.dataset.nanomorphComponentId : undefined
}

// Update the children of elements
// (obj, obj) -> null
function updateChildren (newNode, oldNode) {
  // if (DEBUG) {
  //   console.log(
  //   'updateChildren\nold\n  %s\nnew\n  %s',
  //   oldNode && oldNode.outerHTML,
  //   newNode && newNode.outerHTML
  // )
  // }
  var oldChild, newChild, morphed, oldMatch

  // The offset is only ever increased, and used for [i - offset] in the loop
  var offset = 0

  for (var i = 0; ; i++) {
    oldChild = oldNode.childNodes[i]
    newChild = newNode.childNodes[i - offset]
    // if (DEBUG) {
    //   console.log(
    //   '===\n- old\n  %s\n- new\n  %s',
    //   oldChild && oldChild.outerHTML,
    //   newChild && newChild.outerHTML
    // )
    // }
    // Both nodes are empty, do nothing
    if (!oldChild && !newChild) {
      break

    // There is no new child, remove old
    } else if (!newChild) {
      oldNode.removeChild(oldChild)
      i--

    // There is no old child, add new
    } else if (!oldChild) {
      oldNode.appendChild(newChild)
      offset++

    // Both nodes are the same, morph
    } else if (same(newChild, oldChild)) {
      morphed = walk(newChild, oldChild)
      if (morphed !== oldChild) {
        oldNode.replaceChild(morphed, oldChild)
        offset++
      }

    // Both nodes do not share an ID or a placeholder, try reorder
    } else {
      oldMatch = null

      // Try and find a similar node somewhere in the tree
      for (var j = i; j < oldNode.childNodes.length; j++) {
        if (same(oldNode.childNodes[j], newChild)) {
          oldMatch = oldNode.childNodes[j]
          break
        }
      }

      // If there was a node with the same ID or placeholder in the old list
      if (oldMatch) {
        morphed = walk(newChild, oldMatch)
        if (morphed !== oldMatch) offset++
        oldNode.insertBefore(morphed, oldChild)

      // It's safe to morph two nodes in-place if neither has an ID
      } else if (!newChild.id && !oldChild.id) {
        morphed = walk(newChild, oldChild)
        if (morphed !== oldChild) {
          oldNode.replaceChild(morphed, oldChild)
          offset++
        }

      // Insert the node at the index if we couldn't morph or find a matching node
      } else {
        oldNode.insertBefore(newChild, oldChild)
        offset++
      }
    }
  }
}

function same (a, b) {
  if (a.id) return a.id === b.id
  if (a.isSameNode) return a.isSameNode(b)
  if (a.tagName !== b.tagName) return false
  if (a.type === TEXT_NODE) return a.nodeValue === b.nodeValue
  return false
}

},{"./lib/morph":6,"nanoassert":3}],5:[function(require,module,exports){
module.exports = [
  // attribute events (can be set with attributes)
  'onclick',
  'ondblclick',
  'onmousedown',
  'onmouseup',
  'onmouseover',
  'onmousemove',
  'onmouseout',
  'onmouseenter',
  'onmouseleave',
  'ontouchcancel',
  'ontouchend',
  'ontouchmove',
  'ontouchstart',
  'ondragstart',
  'ondrag',
  'ondragenter',
  'ondragleave',
  'ondragover',
  'ondrop',
  'ondragend',
  'onkeydown',
  'onkeypress',
  'onkeyup',
  'onunload',
  'onabort',
  'onerror',
  'onresize',
  'onscroll',
  'onselect',
  'onchange',
  'onsubmit',
  'onreset',
  'onfocus',
  'onblur',
  'oninput',
  // other common events
  'oncontextmenu',
  'onfocusin',
  'onfocusout'
]

},{}],6:[function(require,module,exports){
var events = require('./events')
var eventsLength = events.length

var ELEMENT_NODE = 1
var TEXT_NODE = 3
var COMMENT_NODE = 8

module.exports = morph

// diff elements and apply the resulting patch to the old node
// (obj, obj) -> null
function morph (newNode, oldNode) {
  var nodeType = newNode.nodeType
  var nodeName = newNode.nodeName

  if (nodeType === ELEMENT_NODE) {
    copyAttrs(newNode, oldNode)
  }

  if (nodeType === TEXT_NODE || nodeType === COMMENT_NODE) {
    if (oldNode.nodeValue !== newNode.nodeValue) {
      oldNode.nodeValue = newNode.nodeValue
    }
  }

  // Some DOM nodes are weird
  // https://github.com/patrick-steele-idem/morphdom/blob/master/src/specialElHandlers.js
  if (nodeName === 'INPUT') updateInput(newNode, oldNode)
  else if (nodeName === 'OPTION') updateOption(newNode, oldNode)
  else if (nodeName === 'TEXTAREA') updateTextarea(newNode, oldNode)

  copyEvents(newNode, oldNode)
}

function copyAttrs (newNode, oldNode) {
  var oldAttrs = oldNode.attributes
  var newAttrs = newNode.attributes
  var attrNamespaceURI = null
  var attrValue = null
  var fromValue = null
  var attrName = null
  var attr = null

  for (var i = newAttrs.length - 1; i >= 0; --i) {
    attr = newAttrs[i]
    attrName = attr.name
    attrNamespaceURI = attr.namespaceURI
    attrValue = attr.value
    if (attrNamespaceURI) {
      attrName = attr.localName || attrName
      fromValue = oldNode.getAttributeNS(attrNamespaceURI, attrName)
      if (fromValue !== attrValue) {
        oldNode.setAttributeNS(attrNamespaceURI, attrName, attrValue)
      }
    } else {
      if (!oldNode.hasAttribute(attrName)) {
        oldNode.setAttribute(attrName, attrValue)
      } else {
        fromValue = oldNode.getAttribute(attrName)
        if (fromValue !== attrValue) {
          // apparently values are always cast to strings, ah well
          if (attrValue === 'null' || attrValue === 'undefined') {
            oldNode.removeAttribute(attrName)
          } else {
            oldNode.setAttribute(attrName, attrValue)
          }
        }
      }
    }
  }

  // Remove any extra attributes found on the original DOM element that
  // weren't found on the target element.
  for (var j = oldAttrs.length - 1; j >= 0; --j) {
    attr = oldAttrs[j]
    if (attr.specified !== false) {
      attrName = attr.name
      attrNamespaceURI = attr.namespaceURI

      if (attrNamespaceURI) {
        attrName = attr.localName || attrName
        if (!newNode.hasAttributeNS(attrNamespaceURI, attrName)) {
          oldNode.removeAttributeNS(attrNamespaceURI, attrName)
        }
      } else {
        if (!newNode.hasAttributeNS(null, attrName)) {
          oldNode.removeAttribute(attrName)
        }
      }
    }
  }
}

function copyEvents (newNode, oldNode) {
  for (var i = 0; i < eventsLength; i++) {
    var ev = events[i]
    if (newNode[ev]) {           // if new element has a whitelisted attribute
      oldNode[ev] = newNode[ev]  // update existing element
    } else if (oldNode[ev]) {    // if existing element has it and new one doesnt
      oldNode[ev] = undefined    // remove it from existing element
    }
  }
}

function updateOption (newNode, oldNode) {
  updateAttribute(newNode, oldNode, 'selected')
}

// The "value" attribute is special for the <input> element since it sets the
// initial value. Changing the "value" attribute without changing the "value"
// property will have no effect since it is only used to the set the initial
// value. Similar for the "checked" attribute, and "disabled".
function updateInput (newNode, oldNode) {
  var newValue = newNode.value
  var oldValue = oldNode.value

  updateAttribute(newNode, oldNode, 'checked')
  updateAttribute(newNode, oldNode, 'disabled')

  if (newValue !== oldValue) {
    oldNode.setAttribute('value', newValue)
    oldNode.value = newValue
  }

  if (newValue === 'null') {
    oldNode.value = ''
    oldNode.removeAttribute('value')
  }

  if (!newNode.hasAttributeNS(null, 'value')) {
    oldNode.removeAttribute('value')
  } else if (oldNode.type === 'range') {
    // this is so elements like slider move their UI thingy
    oldNode.value = newValue
  }
}

function updateTextarea (newNode, oldNode) {
  var newValue = newNode.value
  if (newValue !== oldNode.value) {
    oldNode.value = newValue
  }

  if (oldNode.firstChild && oldNode.firstChild.nodeValue !== newValue) {
    // Needed for IE. Apparently IE sets the placeholder as the
    // node value and vise versa. This ignores an empty update.
    if (newValue === '' && oldNode.firstChild.nodeValue === oldNode.placeholder) {
      return
    }

    oldNode.firstChild.nodeValue = newValue
  }
}

function updateAttribute (newNode, oldNode, name) {
  if (newNode[name] !== oldNode[name]) {
    oldNode[name] = newNode[name]
    if (newNode[name]) {
      oldNode.setAttribute(name, '')
    } else {
      oldNode.removeAttribute(name)
    }
  }
}

},{"./events":5}],7:[function(require,module,exports){
// Copyright Joyent, Inc. and other Node contributors.
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to permit
// persons to whom the Software is furnished to do so, subject to the
// following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN
// NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
// DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
// OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE
// USE OR OTHER DEALINGS IN THE SOFTWARE.

'use strict';

// If obj.hasOwnProperty has been overridden, then calling
// obj.hasOwnProperty(prop) will break.
// See: https://github.com/joyent/node/issues/1707
function hasOwnProperty(obj, prop) {
  return Object.prototype.hasOwnProperty.call(obj, prop);
}

module.exports = function(qs, sep, eq, options) {
  sep = sep || '&';
  eq = eq || '=';
  var obj = {};

  if (typeof qs !== 'string' || qs.length === 0) {
    return obj;
  }

  var regexp = /\+/g;
  qs = qs.split(sep);

  var maxKeys = 1000;
  if (options && typeof options.maxKeys === 'number') {
    maxKeys = options.maxKeys;
  }

  var len = qs.length;
  // maxKeys <= 0 means that we should not limit keys count
  if (maxKeys > 0 && len > maxKeys) {
    len = maxKeys;
  }

  for (var i = 0; i < len; ++i) {
    var x = qs[i].replace(regexp, '%20'),
        idx = x.indexOf(eq),
        kstr, vstr, k, v;

    if (idx >= 0) {
      kstr = x.substr(0, idx);
      vstr = x.substr(idx + 1);
    } else {
      kstr = x;
      vstr = '';
    }

    k = decodeURIComponent(kstr);
    v = decodeURIComponent(vstr);

    if (!hasOwnProperty(obj, k)) {
      obj[k] = v;
    } else if (isArray(obj[k])) {
      obj[k].push(v);
    } else {
      obj[k] = [obj[k], v];
    }
  }

  return obj;
};

var isArray = Array.isArray || function (xs) {
  return Object.prototype.toString.call(xs) === '[object Array]';
};

},{}],8:[function(require,module,exports){
// Copyright Joyent, Inc. and other Node contributors.
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to permit
// persons to whom the Software is furnished to do so, subject to the
// following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN
// NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
// DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
// OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE
// USE OR OTHER DEALINGS IN THE SOFTWARE.

'use strict';

var stringifyPrimitive = function(v) {
  switch (typeof v) {
    case 'string':
      return v;

    case 'boolean':
      return v ? 'true' : 'false';

    case 'number':
      return isFinite(v) ? v : '';

    default:
      return '';
  }
};

module.exports = function(obj, sep, eq, name) {
  sep = sep || '&';
  eq = eq || '=';
  if (obj === null) {
    obj = undefined;
  }

  if (typeof obj === 'object') {
    return map(objectKeys(obj), function(k) {
      var ks = encodeURIComponent(stringifyPrimitive(k)) + eq;
      if (isArray(obj[k])) {
        return map(obj[k], function(v) {
          return ks + encodeURIComponent(stringifyPrimitive(v));
        }).join(sep);
      } else {
        return ks + encodeURIComponent(stringifyPrimitive(obj[k]));
      }
    }).join(sep);

  }

  if (!name) return '';
  return encodeURIComponent(stringifyPrimitive(name)) + eq +
         encodeURIComponent(stringifyPrimitive(obj));
};

var isArray = Array.isArray || function (xs) {
  return Object.prototype.toString.call(xs) === '[object Array]';
};

function map (xs, f) {
  if (xs.map) return xs.map(f);
  var res = [];
  for (var i = 0; i < xs.length; i++) {
    res.push(f(xs[i], i));
  }
  return res;
}

var objectKeys = Object.keys || function (obj) {
  var res = [];
  for (var key in obj) {
    if (Object.prototype.hasOwnProperty.call(obj, key)) res.push(key);
  }
  return res;
};

},{}],9:[function(require,module,exports){
'use strict';

exports.decode = exports.parse = require('./decode');
exports.encode = exports.stringify = require('./encode');

},{"./decode":7,"./encode":8}],10:[function(require,module,exports){
var qs = require('querystring')

//get the data for this form, including the clicked button.
exports.getData = function (form, button) {
  var data = {}
  ;[].forEach.call(form.querySelectorAll('input'), function (input) {
    if(input.name) data[input.name] = input.value
  })
  if(button && button.name)
    data[button.name] = button.value

  return data
}

exports.submit = function (form, button, cb) {
  var data = exports.getData(form, button)
  var data_encoded = qs.encode(data)
  var xhr = new XMLHttpRequest()
  xhr.open(form.method || 'POST', form.action || window.location)
  xhr.setRequestHeader('content-type', 'application/x-www-form-urlencoded')
  xhr.onload = function (ev) {
    cb(null, xhr.response)
  }
  xhr.onerror = function (ev) {
    cb(new Error(xhr.status+':'+xhr.statusText))
  }
  xhr.send(data_encoded)
}






},{"querystring":9}]},{},[1]);
