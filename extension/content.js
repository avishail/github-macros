/*
    TODO
    1. basic add flow with some client side validation
    2. use local storage to store the first suggestions batch. Load them
    3. store in local storage the last X macros you used. Show them
    4. settings - configure more sites and set the action keys
    5. fetch and show small footer from the server? Ask Sivan
    6. test load more
*/

const macroNamePrefix = 'github-macros-';
const loadMorePixelsBeforeScrollEnd = 200;
const hostnameToPattern = new Map();
const contentCache = new Map();
const macroNameToUrl = new Map();
const tooltipsCache = new Map();
const targetIdToTarget = new Map();

getElement = function(targetId, id) {
    return $('#' + targetId + '_' + id)[0]
}

showSearchSpinner = function(targetId) {
    getElement(targetId, 'searchSpinner').style.display = 'inline';
    getElement(targetId, 'searchIcon').style.display = 'none';
}

hideSearchSpinner = function(targetId) {
    getElement(targetId, 'searchSpinner').style.display = 'none';
    getElement(targetId, 'searchIcon').style.display = 'inline';
}

initSearchInput = function (targetId) {
    const searchInput = getElement(targetId, 'macroSearchInput');
    searchInput.addEventListener('input', updateValue);

    let inputTimerId = null;

    const ongoingRequests = new Map();

    function updateValue(e) {
        // clear previous input driven scheduling since we have a new value to process
        if (inputTimerId != null) {
            clearTimeout(inputTimerId);
            inputTimerId = null;
        }

        const searchText = searchInput.value;

        // try to serve from cache
        if (updateUIFromCache(targetId, searchText)) {
            hideSearchSpinner(targetId);
            return;
        }

        // if we already have an ongoing request, do nothing and let that
        // request to complete
        if (ongoingRequests.has(searchText)) {
            return;
        }

        showSearchSpinner(targetId)

        inputTimerId = setTimeout(
            () => {
                ongoingRequests.set(searchText, true);
                fetchContent(
                    targetId,
                    searchText,
                    0,
                    (content) => {
                        updateCacheWithNewPageContent(searchText, content);
                        
                        ongoingRequests.delete(searchText);

                        // check if in the mean time, the user changed the input
                        if (searchText !== searchInput.value) {
                            return;
                        }

                        updateUIWithContent(targetId, content, true);
                        hideSearchSpinner(targetId);
                    },
                    () => { 
                        ongoingRequests.delete(searchText);
                        if (searchText === searchInput.value) {
                            hideSearchSpinner(targetId);
                        }
                    }
                )
            },
            500,
        );
    }
}

/*
 * get the actual text we need to place inside the text area for the macro to
 * appear. Each site might have its own format.
 */ 
getInjectedMacro = function(name, url) {
    const pattern = hostnameToPattern.get(location.hostname);
    return pattern.replace('$name', macroNamePrefix+name).replace('$url', url);
}

selectMacro = function(targetId, macro) {
    target = targetIdToTarget.get(targetId);
    const selectionIndex = target.selectionStart - 1;
    const macroToInject = getInjectedMacro(macro["name"], macro["url"]);
    const newValue = target.value.slice(0, target.selectionStart - 1) + macroToInject + target.value.slice(target.selectionStart);
    target.value = newValue;
    tooltipsCache.get(target)['tooltip'].close();

    // make the origin target focused and place the cursor where it used to be
    target.focus();
    target.selectionStart = selectionIndex + macroToInject.length;
    target.selectionEnd = target.selectionStart;

    $.post( "https://us-central1-github-macros.cloudfunctions.net/mutate/", { type: "use", name: macro["name"] } );
}

/*
 * Loads results from cache and updates the UI. Returns true if there were
 * results in cache.
 */
updateUIFromCache = function(targetId, searchText) {
    if (!contentCache.has(searchText)) {
        return false;
    }

    updateUIWithContent(targetId, contentCache.get(searchText), true)

    return true;
}

fetchContent = function(targetId, searchText, pageToFetch, onFinishCallback, onErrorCallback) {
    const url = new URL('https://us-central1-github-macros.cloudfunctions.net/query/')

    if (searchText !== '') {
        url.searchParams.append('type', 'search')
        url.searchParams.append('text', searchText)
        url.searchParams.append('page', pageToFetch)
    } else {
        url.searchParams.append('type', 'suggestion')
        url.searchParams.append('page', pageToFetch)
    }

    $.ajax({
        url: url,
        success: function(responseText) {
            results = JSON.parse(responseText);
            onFinishCallback(results);
        },
        error: function() {
            if (onErrorCallback) {
                onErrorCallback();
            }
        },
    });
}

updateCacheWithNewPageContent = function(searchText, content) {
    if (!contentCache.has(searchText)) {
        contentCache.set(searchText, {'data': [], 'has_more': false, 'next_page': 0})
    }

    cachedContent = contentCache.get(searchText)

    const items = content['data'];
    for (var i = 0; i < items.length; i++) {
        const item = items[i];
        cachedContent['data'].push(item);
        macroNameToUrl.set(item['name'], item['url']);
    }

    cachedContent['next_page'] = cachedContent['next_page'] + 1;
    cachedContent['has_more'] = content['has_more'];

}

updateUIWithContent = function (targetId, content, isFirstPage) {
    leftMacroDiv = getElement(targetId, 'leftMacros');
    rightMacroDiv = getElement(targetId, 'rightMacros');

    if (isFirstPage) {
        leftMacroDiv.innerHTML = '';
        rightMacroDiv.innerHTML = '';                            
    }

    getElement(targetId, 'moreResultsSpinner').style.display = content['has_more'] ? 'inline' : 'none';

    let divs = [leftMacroDiv, rightMacroDiv];
    const items = content['data'];
    for (var i = 0; i < items.length; i++) {
        const item = items[i];

        const image = document.createElement('img');
        image.style.width = '100%';
        image.style.marginTop = '5px';
        image.style.marginBottom = '5px';
        image.style.borderRadius = '5px';
        image.src = item['url'];
        image.title = item['name'];
        image.onclick = function() {selectMacro(targetId, item);};
        image.onerror = function () {
            $.post( "https://us-central1-github-macros.cloudfunctions.net/mutate/", { type: "report", name: macro["name"] } );
        };
        divs[i % 2].appendChild(image);
    }
}

createTooltip = function(targetId, target) {
    const idPrefix = targetId + '_';
    const magGlassSrc = chrome.runtime.getURL('img/icons/mglass-4x.png');
    const plusSrc = chrome.runtime.getURL('img/icons/plus-4x.png');

    var fetchWasCalled = false;

    return new jBox(
        'Tooltip',
        {
            target: target,
            addClass: 'tooltipBorder',
            width: '300px',
            height: '400px',
            closeOnClick: 'body',
            closeOnEsc: true,
            position: {
                x: 'left',
                y: 'top'
            },
            outside: 'y',
            pointer: 'left:20',
            offset: {
                x: 25
            },
            onCreated: function () {
                initNewTooltip(targetId);
            },
            onOpen: function () {
                if (contentCache.has('')) {
                    updateUIFromCache(targetId, '')
                    // TODO if storgae is too old, trigger a refresh
                    return;
                }

                if (fetchWasCalled) {
                    return;
                }

                fetchWasCalled = true;

                fetchContent(
                    targetId,
                    '',
                    0,
                    (content) => {
                        updateCacheWithNewPageContent('', content);
                        updateUIWithContent(targetId, content, true);
                        // TODO update local storage
                    },
                    () => { fetchWasCalled = false; },
                )
            },
            onOpenComplete: function () {
                getElement(targetId, 'macroSearchInput').focus();
            },
            onClose: function () {
                handleCloseTooltip(targetId);
            },
            content:`
            <div style="width: 100%; height: 100%; position: absolute; top: 0px; left: 0px; right: 0px; bottom: 0px; overflow: hidden">
                <div style="display: flex; flex-direction: row; height: 55px;">
                    <div style="display: flex; flex-direction: row; width: 100%; height: auto; margin: 10px; border-width: 1px; border-style: solid none solid solid; border-color: #234C87; border-radius: 18px; overflow: auto">
                        <div style="display: flex; margin-left: 10px; width: 15px; height: 15px; justify-content: center; align-items: center; height: 100%; ">
                            <img id="${idPrefix}searchIcon" style="width: 15px; height: 15px;" src="${magGlassSrc}" />
                            <div id="${idPrefix}searchSpinner" style="display: none;" class="gh-macros-loader"></div>
                        </div>
                        <div style="display: flex; justify-content: center; align-items: center; height: 100%; flex-grow: 1;">
                            <input class="gh-macros-no-outline" type="text" id="${idPrefix}macroSearchInput" style="width: 100%; padding-left: 12px;">
                        </div>
                        <div style="display: flex; justify-content: center; align-items: center; height: 100%; width: 70px; background-color: #234C87; border-radius: 10px 0 0 10px;">
                            <text style="font-family: 'Pragati Narrow'; font-weight: 700; color: white; user-select: none;">Search</text>
                        </div>
                    </div>
                </div>
                <div id="${idPrefix}macrosSection" class="gh-macros-hideScrollbar" style="width: 100%; height: 340px; overflow: auto; display: flex; flex-direction: column;">
                    <div style="width: 100%; display: flex; flex-direction: row; flex:1; padding-top: 5px">
                        <div id="${idPrefix}leftMacros" style="width: 55%; margin-left: 10px; margin-right: 5px;">
                        </div>
                        <div id="${idPrefix}rightMacros" style="width: 45%; margin-left: 5px; margin-right: 10px;">
                        </div>
                    </div>
                    <div id="${idPrefix}moreResultsSpinner" style="width: 100%; height: 40px;flex-shrink: 0; align-items: center; display: flex;">
                        <div class="gh-macros-loader" style="font-size: 2px;"></div>
                    </div>
                </div>
                <div id="${idPrefix}addNewMacro" class="gh-macros-box-shadow gh-macros-hoover-bg" style="z-index: 100; display: flex; justify-content: center; align-items: center; position: absolute; right: 16px; bottom: 16px; width: 30px; height: 30px; background-color: #234C87; border-radius: 15px;">
                    <img style="width: 7px; height: 7px" src="${plusSrc}" />
                </div>
            </div>
            `,
        },
    )
}

makeid = function(length) {
    var result           = '';
    const characters       = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    const charactersLength = characters.length;
    for ( var i = 0; i < length; i++ ) {
        result += characters.charAt(Math.floor(Math.random() * charactersLength));
    }
    return result;
}

createTooltipMeta = function(target) {
    const targetId = makeid(10)
    const tooltip = createTooltip(targetId, target);

    const newTooltipMeta = {
        'targetId': targetId,
        'tooltip': tooltip,
    };

    return newTooltipMeta
}

handleCloseTooltip = function(targetId) {
    getElement(targetId, 'macroSearchInput').value = '';
    hideSearchSpinner(targetId);
    getElement(targetId, 'leftMacros').innerHTML = '';
    getElement(targetId, 'rightMacros').innerHTML = '';
    getElement(targetId, 'moreResultsSpinner').style.display = 'inline';
    targetIdToTarget.get(targetId).focus();
}

initMacrosScrollLogic = function(targetId) {
    macrosSection = getElement(targetId, 'macrosSection');
    // init scroll of the macros section
    macrosSection.addEventListener(
        "onwheel" in document ? "wheel" : "mousewheel",
        e => {
            e.wheel = e.deltaY ? -e.deltaY : e.wheelDelta/40;
            macrosSection.scrollTop -= e.wheel;
        },
    );

    var loadingMore = false;
    macrosSection.addEventListener('scroll', function(e) {
        if (loadingMore) {
            return
        }

        loadingMore = true;

        if (macrosSection.offsetHeight + macrosSection.scrollTop > macrosSection.scrollHeight - loadMorePixelsBeforeScrollEnd) {
            fetchNextPage(targetId, () => { loadingMore = false; })
        }
      });
}

initAddNewMacroLogic = function(targetId) {
    addNewMacro = getElement(targetId, 'addNewMacro');
    addNewMacro.onmouseover = function() {addNewMacro.style.background='#526683'};
    addNewMacro.onmouseout = function() {addNewMacro.style.background='#234C87'};
    addNewMacro.onclick = function() {alert('click')};
}

initNewTooltip = function(targetId) {
    initSearchInput(targetId);
    initMacrosScrollLogic(targetId);
    initAddNewMacroLogic(targetId);
}

openTooltip = function(target) {
    var tooltipMeta;
    if (tooltipsCache.has(target)) {
        tooltipMeta = tooltipsCache.get(target);
    } else {
        tooltipMeta = createTooltipMeta(target);
        tooltipsCache.set(target, tooltipMeta);
        targetIdToTarget.set(tooltipMeta['targetId'], target);
    }
    tooltipMeta["tooltip"].open()
}    

requestMacroOnPattern = function(target) {
    const value = target.value.slice(0, target.selectionStart - 1);
    var index = value.length - 1;
    while (index >= 0 && value[index] != ' ' && value[index] != '$') {
        index--;
    }

    if (value[index] !== '$') {
        return;
    }

    const macroName =  value.slice(index+1, target.selectionStart)

    if (macroNameToUrl.has(macroName)) {
        const macroURL = macroNameToUrl.get(macroName);
        if (macroURL == null) {
            return;
        }
        injectMacroPattern(target, macroName, macroURL);
        return;
    }

    const url = new URL('https://us-central1-github-macros.cloudfunctions.net/query/')
    url.searchParams.append('type', 'get')
    url.searchParams.append('text', macroName)

    $.ajax({
        url: url,
        success: function(responseText) {
            const results = JSON.parse(responseText);
            if (results['data'].length == 0) {
                macroNameToUrl.set(macroName, null);
            } else {
                macroURL = results['data'][0]['url'];
                macroNameToUrl.set(macroName, macroURL);
                injectMacroPattern(target, macroName, macroURL);
            }
        },
    });
}

injectMacroPattern = function(target, name, url) {
    var value = target.value; 
    var selectionStart = target.selectionStart;

    const pattern = '$' + name + '$';
    const injectedMacro = getInjectedMacro(name, url)
    
    var index = value.indexOf(pattern);
    while (index !== -1) {
        if (index < selectionStart) {
            selectionStart -= pattern.length
            selectionStart += injectedMacro.length
        }
        
        value = value.replace(pattern, injectedMacro);
        var index = value.indexOf(pattern);
    }
    
    target.value = value
    target.selectionStart = selectionStart
    target.selectionEnd = selectionStart
}

initKeyboardListeners = function() {
    var inputTimerId;
    document.onkeydown = function(ev) {
        if (!hostnameToPattern.has(location.hostname)) {
            return
        }

        activeElement = document.activeElement
        // TODO needs to be configured whether we wanna do it also
        // for regular input + exlusion of our input fields
        if (!(activeElement instanceof HTMLTextAreaElement)) {
            return
        }

        // we open only for ! that is the beginning of a new word
        if (ev.key === '!') {
            if (inputTimerId) {
                clearTimeout(inputTimerId)
            }
            inputTimerId = setTimeout(
                () => {
                    inputTimerId = null;
                    const text = activeElement.value
                    const pos = activeElement.selectionStart
                    if (
                        text[pos-1] === '!' && // last typed char was !
                        (pos === 1 || text[pos-2] === ' ' || text[pos-2] === '\n') && // left to the !, it is a new word
                        (pos === text.length || text[pos] === ' ' || text[pos] === '\n')) { // right to the !, it is a new word
                        openTooltip(activeElement);
                    }
                },
                250);
        }

        if (ev.key === '$') {
            setTimeout(() => requestMacroOnPattern(activeElement), 0)
        }
    };
}

loadSuggestionsFromStorage = function() {
    // chrome.storage.sync.get(['foo', 'bar'], function(items) {
    //     console.log('Settings retrieved', items);
    //   });
}

initSupportedHostNames = function() {
    // TODO load from storage with a callback to trigger the rest of the init
    hostnameToPattern.set('github.com', '![$name]($url)');
}

processGithubMacroImages = function() {
    $('img').each(function() {
        if (this.alt && this.alt.startsWith(macroNamePrefix)) {
            this.title = this.alt.slice(macroNamePrefix.length)
        }
      });
}

window.onload = function() {
    initSupportedHostNames();

    if (!hostnameToPattern.has(location.hostname)) {
        return;
    }

    initKeyboardListeners();
    // loadSuggestionsFromStorage()
    // chrome.storage.sync.set({'foo': 'hello', 'bar': 'hi'}, function() {
    //     console.log('Settings saved');
    // });
    processGithubMacroImages();
}