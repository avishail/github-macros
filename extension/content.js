/*
    TODO
    2. keep track of the last 100 usages, store it in local storage, load it during boot and place the top X 
       need to see how we make sure we will know how to load more suggestions nicely
       with top usages - DONE but needs to be tested
    4. fetch and show small footer from the server? Ask Sivan
    5. test load more
*/

class ErrorCodes {
    static Success = 0
    static EmptyName = 1
	static NameContainsSpaces = 2
	static NameAlreadyExist = 3
	static EmptyURL = 4
	static InvalidURL = 5
	static URLHostnameNotSupported = 6
	static FileIsTooBig = 7
	static FileFormatNotSupported = 8
    static TransientError = 9
}

Object.freeze(ErrorCodes); 

const gVersion = "1.0.0"

const numberOfTopUsagesToDisplay = 20;
const maxTopUsagesToStore = 1;
const maxSuggestionsFreshnessDuration = 60 * 60 * 24 * 1000;
const macroNamePrefix = 'github-macros-';
const loadMorePixelsBeforeScrollEnd = 200;
const hostnameToPattern = new Map();
const contentCache = new Map();
const macroNameToUrl = new Map();
const tooltipsCache = new Map();
const targetIdToTarget = new Map();
const macroTopUsages = [];
const nameToTopUsage = new Map();

const gGifIconSrc = chrome.runtime.getURL('img/icons/gif.png');
const gMagGlassIconSrc = chrome.runtime.getURL('img/icons/mglass-4x.png');
const gPlusIconSrc = chrome.runtime.getURL('img/icons/plus-4x.png');
const gXIconSrc = chrome.runtime.getURL('img/icons/x-4x.png');
const gVIconSrc = chrome.runtime.getURL('img/icons/v-4x.png');

closeTooltipOnClickOutside = true;

catchAndLog = function(f) {
    return function() {
        try {
            return f.apply(this, arguments);
        } catch (e) {
            $.post( 
                "https://us-central1-github-macros.cloudfunctions.net/client_errors/",
                { version: gVersion, stacktrace: e.stack},
            );
            console.log(e.stack)
        }
    }
}

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
    searchInput.addEventListener('input', catchAndLog(updateValue));

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
            catchAndLog(
                () => {
                    ongoingRequests.set(searchText, true);
                    fetchContent(
                        targetId,
                        searchText,
                        0,
                        (content) => {
                            updateCacheWithNewContent(searchText, content['data'], true, content['has_more']);
                            
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
            ),    
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

    setTimeout(
        catchAndLog(
            () => {
                updateTopUsages(macro);
                $.post( 
                    "https://us-central1-github-macros.cloudfunctions.net/usage/",
                    {name: macro["name"] },
                );
            }
        ),
        0,
    );
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
        success: catchAndLog(
            function(responseText) {
                results = JSON.parse(responseText);
                onFinishCallback(results);
                // if ('system_message' in results) {
                //     processSystemMessage(results['system_message']);
                // }
                processSystemMessage(
                    {
                        'id': 'sR5td3-43',
                        'snooze_time': 60 * 60 * 24,
                        'total_impressions': 3,
                        'content': 'This is a system message!!!',
                    }
                )
            },
        ),    
        error: catchAndLog(
            function() {
                if (onErrorCallback) {
                    onErrorCallback();
                }
            },
        ),    
    });
}

updateCacheWithNewContent = function(searchText, items, isRealPage, hasMore) {
    if (!contentCache.has(searchText)) {
        contentCache.set(searchText, {'data': [], 'has_more': false, 'next_page': 0})
    }

    cachedContent = contentCache.get(searchText)

    for (var i = 0; i < items.length; i++) {
        const item = items[i];

        if (macroNameToUrl.has(item['name'])) {
            continue;
        }

        cachedContent['data'].push(item);
        macroNameToUrl.set(item['name'], item['url']);
    }

    if (isRealPage) {
        cachedContent['next_page'] = cachedContent['next_page'] + 1;
    }
    cachedContent['has_more'] = hasMore;
}

createSingleMacro = function(targetId, item) {
    const div = document.createElement('div');
    div.style.width = '100%';
    div.style.marginTop = '5px';
    div.style.marginBottom = '5px';
    div.style.borderRadius = '5px';
    div.style.position = 'relative';
    div.onclick = function() {selectMacro(targetId, item);};

    const image = document.createElement('img');
    image.style.width = '100%';
    image.src = item['thumbnail'] || item['gif_thumbnail'] || item['url'];
    image.title = item['name'];
    image.style.display = 'block';
    image.onerror = function () {
        div.parentElement.removeChild(div);
        $.post( "https://us-central1-github-macros.cloudfunctions.net/report/", { name: item["name"] } );
    };

    div.appendChild(image)

    const gifDiv = document.createElement('div');
    const gifIcon = document.createElement('img');
    if (item['is_gif'] && item['thumbnail']) {
        gifDiv.style.position = 'absolute';
        gifDiv.style.width = '100%';
        gifDiv.style.height = '100%';
        gifDiv.style.top = 0;
        gifDiv.style.left = 0;
        gifDiv.style.display = 'flex';
        gifDiv.style.justifyContent = 'center';
        gifDiv.style.alignItems = 'center';

        gifIcon.src = gGifIconSrc;
        gifIcon.style.width = '30px';
        gifIcon.style.aspectRatio = '1.32';

        gifDiv.appendChild(gifIcon);

        div.appendChild(gifDiv)
    }

    if (item['is_gif'] && item['thumbnail']) {
        gifDiv.onmouseenter = function() {
            gifIcon.style.visibility = 'hidden';
            image.src = item['gif_thumbnail'] || item['url'];

        }

        gifDiv.onmouseleave = function() {
            gifIcon.style.visibility = 'visible';
            image.src = item['thumbnail'];
        }
    }

    return div
}

updateUIWithContent = function (targetId, content, isFirstPage) {
    leftMacroDiv = getElement(targetId, 'leftMacros');
    rightMacroDiv = getElement(targetId, 'rightMacros');

    if (isFirstPage) {
        leftMacroDiv.innerHTML = '';
        rightMacroDiv.innerHTML = '';                            
    }

    getElement(targetId, 'moreResultsSpinner').style.display = content['has_more'] ? 'inline' : 'none';

    let divs = rightMacroDiv.childElementCount >= leftMacroDiv.childElementCount 
        ? [leftMacroDiv, rightMacroDiv] 
        : [rightMacroDiv, leftMacroDiv];
    const items = content['data'];
    for (var i = 0; i < items.length; i++) {
        divs[i % 2].appendChild(createSingleMacro(targetId, items[i]));
    }
}

createTooltip = function(targetId, target) {
    const idPrefix = targetId + '_';
    var fetchWasCalled = false;
    var resizeObserver;

    return new jBox(
        'Tooltip',
        {
            target: target,
            addClass: 'tooltipBorder',
            width: '300px',
            height: '400px',
            closeOnEsc: true,
            closeOnClick: 'body',
            position: {
                x: 'left',
                y: 'top'
            },
            outside: 'y',
            pointer: 'left:20',
            offset: {
                x: 25
            },
            onCreated: catchAndLog(
                function () {
                    initNewTooltip(targetId);
                },
            ),
            onOpen: catchAndLog(
                function () {
                    if (fetchWasCalled) {
                        return;
                    }

                    fetchWasCalled = true;

                    if (contentCache.has('')) {
                        updateUIFromCache(targetId, '')

                        // TODO remove this once we will have enough initial images
                        // this will trigger a silent update in the background so next
                        // time the user opens the tooltip, the new content will be there
                        fetchContent(
                            targetId,
                            '',
                            0,
                            (content) => {
                                fetchWasCalled = false;
                                updateCacheWithNewContent('', content['data'], true, content['has_more']);
                                updateUIWithContent(targetId, contentCache.get(''), true);
                                chrome.storage.sync.set({
                                    'suggestions': JSON.stringify(content),
                                    'suggestions_freshness': Date.now().toString()
                                });
                            },
                            () => { fetchWasCalled = false; },
                        )

                        return;
                    }

                    fetchContent(
                        targetId,
                        '',
                        0,
                        (content) => {
                            fetchWasCalled = false;
                            updateCacheWithNewContent('', content['data'], true, content['has_more']);
                            updateUIWithContent(targetId, content, true);
                            chrome.storage.sync.set({
                                'suggestions': JSON.stringify(content),
                                'suggestions_freshness': Date.now().toString()
                            });
                        },
                        () => { fetchWasCalled = false; },
                    )
                },
            ),
            onOpenComplete: catchAndLog(
                function () {
                    maybeShowSystemMessage(targetId);
                    const tooltipDiv = getElement(targetId, 'macroMainWindow').parentNode;
                    const macrosSection = getElement(targetId, 'macrosSection');
                    resizeObserver = new ResizeObserver(entries => {
                        macrosSection.style.height = tooltipDiv.offsetHeight - 55 + "px";
                    });
                    resizeObserver.observe(tooltipDiv);
                    // since tooltip might be shorter, we need to let the macros section to have
                    // the rest of the tooltip's space
                    //getElement(targetId, 'macrosSection').style.height = getElement(targetId, 'macroMainWindow').parentNode.offsetHeight - 55 + "px";
                    getElement(targetId, 'macroSearchInput').focus();
                },
            ),    
            onClose: catchAndLog(
                function () {
                    if (resizeObserver) {
                        resizeObserver.disconnect();
                    }
                    handleCloseTooltip(targetId);
                },
            ),    
            content:`
            <div id="${idPrefix}macroMainWindow" style="width: 100%; height: 100%; position: absolute; top: 0px; left: 0px; right: 0px; bottom: 0px; overflow: hidden">
                <div style="display: flex; flex-direction: row; height: 55px;">
                    <div style="display: flex; flex-direction: row; width: 100%; height: auto; margin: 10px; border-width: 1px; border-style: solid none solid solid; border-color: #234C87; border-radius: 18px; overflow: auto">
                        <div style="display: flex; margin-left: 10px; width: 15px; height: 15px; justify-content: center; align-items: center; height: 100%; ">
                            <img id="${idPrefix}searchIcon" style="width: 15px; height: 15px;" src="${gMagGlassIconSrc}" />
                            <div id="${idPrefix}searchSpinner" style="display: none;" class="gh-macros-loader gh-macros-dark-loader"></div>
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
                        <div class="gh-macros-loader gh-macros-dark-loader" style="font-size: 2px;"></div>
                    </div>
                </div>
                <div id="${idPrefix}openMacroCreationButton" class="gh-macros-box-shadow gh-macros-hoover-bg" style="z-index: 1; display: flex; justify-content: center; align-items: center; position: absolute; right: 16px; bottom: 16px; width: 30px; height: 30px; background-color: #234C87; border-radius: 15px;">
                    <img style="width: 7px; height: 7px" src="${gPlusIconSrc}" />
                </div>
                <div id="${idPrefix}addNewMacroWrapper" style="z-index: 2; display: flex; flex-direction: column; position: absolute; bottom: 0; left:0; right: 0; width: 100%; height: 0; align-items: end">
                    <div id="${idPrefix}addNewMacroCloseMargins" style="height: 150px; width: 100%"></div>
                    <div style="display: flex; flex-direction: column; height: 100%; width: 100%; background-color: #234C87; border-radius: 10px 10px 0px 0px;">
                        <div style="text-align: right; margin:10px 10px 0 10px;">
                            <text id="${idPrefix}addNewMacroCloseButton" style="font-family: 'Pragati Narrow'; font-weight: 700; font-size: 12px; color: white; user-select: none;">X</text>
                        </div>

                        <div id="${idPrefix}addNewMacroEdit" style="display: flex; flex-direction: column; width: 100%; height: 100%">
                            <div style="margin: 0 15px 15px 15px">
                                <text style="font-family: 'Pragati Narrow'; font-weight: 400; font-size: 12px; color: white; user-select: none;">Name</text>
                                <div style="border-radius: 5px; background-color: #ffffff; overflow: hidden;">
                                    <input class="gh-macros-no-outline" type="text" id="${idPrefix}newMacroName" style="width: 100%; margin: 4px;">
                                </div>    
                            </div>
                            <div style="margin: 0 15px 15px 15px">
                                <text style="font-family: 'Pragati Narrow'; font-weight: 400; font-size: 12px; color: white; user-select: none;">URL</text>
                                <div style="border-radius: 5px; background-color: #ffffff; overflow: hidden;">
                                    <input class="gh-macros-no-outline" type="text" id="${idPrefix}newMacroURL" style="width: 100%; margin: 4px;">
                                </div>    
                            </div>
                            <div id="${idPrefix}addNewMacroButton" style="display: flex; border-radius: 4px; margin: 15px 15px 15px 15px; background-color: #FFFFFF; justify-content: center;">
                                <text style="font-family: 'Pragati Narrow'; font-weight: 700; font-size: 12px; color: black; user-select: none; margin: 4px;">Add to the pile</text>
                            </div>
                            <div style="display: flex; flex-grow: 1; flex-direction: column; margin: 0px 15px 15px 15px;">
                                <div id="${idPrefix}addNewMacroSpinner" class="gh-macros-light-loader gh-macros-loader" style="display: none; font-size: 2px;"></div>
                                <div id="${idPrefix}addNewMacroError" style="display: none; flex-direction: column; align-items: center; text-align: center;">
                                    <img src="${gXIconSrc}" style="width: 12px; height: 12px"/>
                                    <div id="${idPrefix}addNewMacroErrorMessage" class="gh-macros-add-new-error-message">
                                    </div>
                                </div>
                            </div>
                        </div> 
                        <div id="${idPrefix}addNewMacroSuccess" style="display: none;flex-direction: column;width: 100%;height: 100%;align-items: center;justify-content: center;">
                            <img src="${gVIconSrc}" style="width: 34px;height: 25px;">
                            <text style="font-family: 'Pragati Narrow';font-weight: 400;font-size: 16px;color: white;user-select: none;margin: 20;text-align: center;">It was added succesfully to the pile and you can start using it! Thanks for contributing :)</text>
                        </div>   
                    </div>  
                </div>
                <div id="${idPrefix}systemMessageWrapper" style="z-index: 2; display: none; flex-direction: column; position: absolute; bottom: 0; left:0; right: 0; width: 100%; height: 0; justify-content: end">
                    <div id="${idPrefix}systemMessageCloseMargins" style="height: 150px; width: 100%"></div>
                    <div style="display: flex; flex-direction: column; height: auto; width: 100%; background-color: #234C87; border-radius: 10px 10px 0px 0px;">
                        <div style="text-align: right; margin:10px 10px 0 10px;">
                            <text id="${idPrefix}systemMessageCloseButton" style="font-family: 'Pragati Narrow'; font-weight: 700; font-size: 12px; color: white; user-select: none;">X</text>
                        </div>

                        <div id="${idPrefix}systemMessageContent" style="display: flex; padding: 0px 8px 8px 8px; flex-direction: column; width: 100%; height: 100%; color: white;">
                        </div> 
                    </div>  
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
        'target_id': targetId,
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
    hideAddMacroUI(targetId);
    dismissSystemMessage(targetId, null, false);
    targetIdToTarget.get(targetId).focus();
}

initMacrosScrollLogic = function(targetId) {
    macrosSection = getElement(targetId, 'macrosSection');
    // init scroll of the macros section
    macrosSection.addEventListener(
        "onwheel" in document ? "wheel" : "mousewheel",
        catchAndLog(
            e => {
                e.wheel = e.deltaY ? -e.deltaY : e.wheelDelta/40;
                macrosSection.scrollTop -= e.wheel;
            },
        ),    
    );

    var loadingMore = false;
    const searchInput = getElement(targetId, 'macroSearchInput');

    macrosSection.addEventListener(
        'scroll',
        catchAndLog(
            function(e) {
                if (loadingMore) {
                    return
                }

                searchText = searchInput.text || '';

                if (!(contentCache.has(searchText))) {
                    return
                }

                content = contentCache.get(searchText)

                if (!(content['has_more'])) {
                    return
                }

                if (macrosSection.offsetHeight + macrosSection.scrollTop > macrosSection.scrollHeight - loadMorePixelsBeforeScrollEnd) {
                    loadingMore = true;
                    fetchContent(
                        targetId,
                        searchText,
                        content['next_page'],
                        (data) => { 
                            loadingMore = false;
                            updateCacheWithNewContent(searchText, content['data'], true, content['has_more']);
                            // check if in the mean time, the user changed the input
                            if (searchText !== searchInput.value) {
                                return;
                            }

                            updateUIWithContent(targetId, content, true);
                        },
                        () => { loadingMore = false; },
                    )
                }
            },
        ),
    );
}

showAddMacroUI = function(targetId) {
    addNewMacroUI = getElement(targetId, 'addNewMacroWrapper')
    $(addNewMacroUI).animate({'height': '100%'});
}

hideAddMacroUI = function(targetId) {
    addNewMacroUI = getElement(targetId, 'addNewMacroWrapper')
    $(addNewMacroUI).animate({'height': '0%'});

    getElement(targetId, 'addNewMacroSuccess').style.display = 'none';
    getElement(targetId, 'addNewMacroEdit').style.display = 'flex';
    getElement(targetId, 'newMacroName').value = '';
    getElement(targetId, 'newMacroURL').value = '';
    getElement(targetId, 'addNewMacroSpinner').style.display = 'none';
    getElement(targetId, 'addNewMacroError').style.display = 'none';
    getElement(targetId, 'addNewMacroErrorMessage').innerHTML = '';
}

initAddNewMacroLogic = function(targetId) {
    // set up add new macro button
    openMacroCreationButton = getElement(targetId, 'openMacroCreationButton');
    openMacroCreationButton.onmouseover = catchAndLog(function() {openMacroCreationButton.style.background='#526683'});
    openMacroCreationButton.onmouseout = catchAndLog(function() {openMacroCreationButton.style.background='#234C87'});
    openMacroCreationButton.onclick = catchAndLog(function() { showAddMacroUI(targetId); });

    // set up add new macro UI
    getElement(targetId, 'addNewMacroCloseMargins').onclick = catchAndLog(() => {hideAddMacroUI(targetId);});
    getElement(targetId, 'addNewMacroCloseButton').onclick = catchAndLog(() => {hideAddMacroUI(targetId);});

    getElement(targetId, 'addNewMacroButton').onclick = catchAndLog(() => {addNewMacro(targetId);});
    
}

processSystemMessage = function(systemMessage) {
    chrome.storage.sync.get(
        ['system_message'],
        catchAndLog(
            function(items) {
                const prevSystemMessage = ('system_message' in items) ? JSON.parse(items['system_message']) : {'number_of_impressions': 0}
                if (prevSystemMessage['id'] === systemMessage['id']) {
                    return;
                }

                systemMessage['number_of_impressions'] = 0
                systemMessage['impression_time'] = 0
                
                chrome.storage.sync.set(
                    {
                        'system_message': JSON.stringify(systemMessage),
                    },
                );
            },
        ),
    ) 
}

maybeShowSystemMessage = function(targetId) {
    chrome.storage.sync.get(
        ['system_message'],
        catchAndLog(
            function(items) {
                if (!('system_message' in items)) {
                    return
                }
                
                const now = Math.round(Date.now()/1000);

                const systemMessage = JSON.parse(items['system_message']);
                if (systemMessage['number_of_impressions'] === systemMessage['total_impressions']) {
                    return;
                }
            
                const timeFromLastImpression = now - systemMessage['impression_time'];
                if (timeFromLastImpression < systemMessage['snooze_time']) {
                    return;
                }

                getElement(targetId, 'systemMessageCloseMargins').onclick = catchAndLog(() => {dismissSystemMessage(targetId, null, false);});
                getElement(targetId, 'systemMessageCloseButton').onclick = catchAndLog(() => {dismissSystemMessage(targetId, systemMessage, true);});
                
                systemMessageWrapper = getElement(targetId, 'systemMessageWrapper')
                systemMessageWrapper.style.display = 'flex';
                getElement(targetId, 'systemMessageContent').innerHTML = systemMessage['content'];
                $(systemMessageWrapper).animate({'height': '100%'});
            },
        ),
    )    
}

dismissSystemMessage = function(targetId, systemMessage, shouldRecordImpression) {
    if (shouldRecordImpression) {
        systemMessage['number_of_impressions']++;
        systemMessage['impression_time'] = Math.round(Date.now()/1000);
                    
        chrome.storage.sync.set(
            {
                'system_message': JSON.stringify(systemMessage),
            },
        );
    }

    systemMessageWrapper = getElement(targetId, 'systemMessageWrapper')
    $(systemMessageWrapper).animate({'height': '0%'});
    systemMessageWrapper.style.display = 'none';
    getElement(targetId, 'systemMessageContent').innerHTML = '';
}

errCodeToHTML = function(targetId, errCode) {
    switch (errCode) {
        case ErrorCodes.EmptyName:
            return "Name is empty";
        case ErrorCodes.NameContainsSpaces:
            return "Name can't contains spaces";
        case ErrorCodes.NameAlreadyExist:
            return "Name is already taken";
        case ErrorCodes.EmptyURL:
            return "URL is empy"
        case ErrorCodes.InvalidURL:
            return "URL is not valid"
        case ErrorCodes.URLHostnameNotSupported:
            return "Only Github URLs are allowed. Drop the image into the comment box and get its URL from the Preview tab"
        case ErrorCodes.FileIsTooBig:
            return `Image exceeds 10Mb. Please reduce its size and try again. You can use <a target="_blank" href="https://ezgif.com/optimize">this</a> website to do it`
        case ErrorCodes.FileFormatNotSupported:
            return "File format is not supported"
        case ErrorCodes.TransientError:
            return "Something went wrong, please try again later"
    }
}

addNewMacroShowErrorMessage = function(targetId, errCode) {
    getElement(targetId, 'addNewMacroSpinner').style.display = 'none';
    getElement(targetId, 'addNewMacroError').style.display = 'flex';
    getElement(targetId, 'addNewMacroErrorMessage').innerHTML = errCodeToHTML(targetId, errCode);
}

addNewMacroShowSuccessMessage = function(targetId) {
    getElement(targetId, 'addNewMacroEdit').style.display = 'none';
    getElement(targetId, 'addNewMacroSuccess').style.display = 'flex';
}

validateInput = function(macroName, macroURL) {
    if (macroName === '') {
        return ErrorCodes.EmptyName
    }

    if (macroName.includes(' ')) {
        return ErrorCodes.NameContainsSpaces
    }

    if (macroURL === '') {
        return ErrorCodes.EmptyURL
    }

    try {
        url = new URL(macroURL);
    } catch {
        return ErrorCodes.InvalidURL
    }

    return ErrorCodes.Success
}

isGitHubMediaLink = function(macroURL) {
    try {
        url = new URL(macroURL);
        return url.hostname.endsWith('githubusercontent.com');
    } catch {
        return false;
    }
}

var addingNewMacro = false

addNewMacro = function(targetId) {
    if (addingNewMacro) {
        return
    }

    getElement(targetId, 'addNewMacroError').style.display = 'none';
    getElement(targetId, 'addNewMacroSpinner').style.display = 'inline';

    const macroName = getElement(targetId, 'newMacroName').value;
    const macroURL = getElement(targetId, 'newMacroURL').value;

    errCode = validateInput(macroName, macroURL)

    if (errCode != ErrorCodes.Success) {
        addNewMacroShowErrorMessage(targetId, errCode);
        return
    }

    fireAddNewMacroRequest = (url, origURL) => {
        $.post(
            "https://us-central1-github-macros.cloudfunctions.net/add/",
            { 
                name: macroName,
                url: url,
                orig_url: origURL,
            },
            catchAndLog(
                (responseText) => {
                    const response = JSON.parse(responseText);
                    if (response['code'] != ErrorCodes.Success) {
                        addNewMacroShowErrorMessage(targetId, response['code']); 
                        return
                    }
    
                    addNewMacroShowSuccessMessage(targetId);
    
                    const newMacro = response['data'];
    
                    macroNameToUrl.set(macroName, macroURL);
    
                    getElement(targetId, 'macroSearchInput').value = "";
                    getElement(targetId, 'macrosSection').scrollTop = 0;
                    getElement(targetId, 'leftMacros').prepend(
                        createSingleMacro(
                            targetId,
                            newMacro,
                        ),
                    )
                    
                },
            ),
        ).fail(
            catchAndLog(
                () => {
                    addNewMacroShowErrorMessage(targetId, ErrorCodes.TransientError); 
                },
            ),
        ).always(catchAndLog(() => {addingNewMacro = false;})); 
    }

    addingNewMacro = true;

    if (isGitHubMediaLink(macroURL)) {
        fireAddNewMacroRequest(macroURL);
    }

    const authToken = $("input[type='hidden'][data-csrf='true'][class='js-data-preview-url-csrf']" )[0]
    if (!authToken || !authToken.value) {
        fireAddNewMacroRequest(macroURL);
    }

    const boundary = "----WebKitFormBoundary" + makeid(16)

    $.ajax({
        url: 'https://github.com/preview',
        type: 'POST',
        data: `--${boundary}\r\nContent-Disposition: form-data; name=\"text\"\r\n\r\n![github-macros-new-url](${macroURL})\r\n--${boundary}\r\nContent-Disposition: form-data; name=\"authenticity_token\"\r\n\r\n${authToken.value}\r\n--${boundary}--\r\n`,
        headers: {
            "content-type": `multipart/form-data; boundary=${boundary}`,
        },
        success: catchAndLog(
            function (data) {
                const responseImage = $($.parseHTML(data)).find('img')[0]
                if (!responseImage || !responseImage.src) {
                    fireAddNewMacroRequest(macroURL);    
                } else {
                    fireAddNewMacroRequest(responseImage.src, macroURL)
                }
            },
        ),
        fail: catchAndLog(
            function() {
                fireAddNewMacroRequest(macroURL)
            },
        ),
    });     
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
        targetIdToTarget.set(tooltipMeta['target_id'], target);
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
        success: catchAndLog(
            function(responseText) {
                const results = JSON.parse(responseText);
                if (results['data'].length == 0) {
                    macroNameToUrl.set(macroName, null);
                } else {
                    macroURL = results['data'][0]['url'];
                    macroNameToUrl.set(macroName, macroURL);
                    injectMacroPattern(target, macroName, macroURL);
                }
            },
        ),    
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
    document.onkeydown = catchAndLog(
        function(ev) {
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
                    catchAndLog(
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
                    ),
                    250,
                );
            }

            if (ev.key === '$') {
                setTimeout(catchAndLog(() => requestMacroOnPattern(activeElement)), 0)
            }
        },
    );
}

updateTopUsages = function(macro) {
    const macroName = macro['name'];
    if (nameToTopUsage.has(macroName)) {
        topUsage = nameToTopUsage.get(macroName);
        topUsage['usages'] = topUsage['usages'] + 1;
        macroTopUsages.sort((a,b) => {return a['usages'] > b['usages']});
    } else if (nameToTopUsage.size == maxTopUsagesToStore) {
        topUsageToRemove = macroTopUsages.pop();
        nameToTopUsage.delete(topUsageToRemove['name'])
    }

    if (!nameToTopUsage.has(macroName)) {
        const newTopUsage = {
            'name': macroName,
            'url': macro['url'],
            'thumbnail': macro['thumbnail'],
            'is_gif': macro['is_gif'],
            'gif_thumbnail': macro['gif_thumbnail'],
            'usages': 1,
        }
        macroTopUsages.push(newTopUsage);
        nameToTopUsage.set(macroName, newTopUsage);
    }

    chrome.storage.sync.set({'top_usages': JSON.stringify(macroTopUsages)});
}

loadSuggestionsFromStorage = function() {
    chrome.storage.sync.get(
        ['suggestions', 'suggestions_freshness', 'top_usages'],
        catchAndLog(
            function(items) {
                if ('top_usages' in items) {
                    topUsages = JSON.parse(items['top_usages'])
                    for (var i = 0; i < topUsages.length; i++) { 
                        const topUsage = topUsages[i];
                        nameToTopUsage.set(topUsage['name'], topUsage);
                    }

                    macroTopUsages.length = 0;
                    for (const usage of topUsages) {
                        macroTopUsages.push(usage)
                    }

                    updateCacheWithNewContent('', macroTopUsages.slice(0, numberOfTopUsagesToDisplay), false, true)
                }
                
                if (!('suggestions' in items) || !('suggestions_freshness' in items)) {
                    return;
                }

                if (Date.now() - parseInt(items['suggestions_freshness']) > maxSuggestionsFreshnessDuration) {
                    return;
                }

                
                content = JSON.parse(items['suggestions'])
                updateCacheWithNewContent('', content['data'], true, true);
            },
        ),
    );
}

initSupportedHostNames = function() {
    // TODO load from storage with a callback to trigger the rest of the init
    hostnameToPattern.set('github.com', '![$name]($url)');
    hostnameToPattern.set('gist.github.com', '![$name]($url)');
}

// put the macro name (appeared in alt property) as the image's title
processGithubMacroImages = function() {
    $('img').each(function() {
        if (this.alt && this.alt.startsWith(macroNamePrefix)) {
            this.title = this.alt.slice(macroNamePrefix.length)
        }
      });
}

window.onload = catchAndLog(
    function() {
        initSupportedHostNames();

        if (!hostnameToPattern.has(location.hostname)) {
            return;
        }

        initKeyboardListeners();
        loadSuggestionsFromStorage()
        processGithubMacroImages();
    },
)

/*

HOW TO REDUCE ADD TIME BY 60%

1. locate a hidden input contains the auth key in the HTML
<input type="hidden" value="secret-value" data-csrf="true" class="js-data-preview-url-csrf">

$( "input[type='hidden'][data-csrf='true'][class='js-data-preview-url-csrf']" ).next()

2. send and HTTP POST request:

fetch("https://github.com/preview", {
  "headers": {
    "content-type": "multipart/form-data; boundary=----WebKitFormBoundaryiKeP7DQBTsft34cT",
  },
  "body": "------WebKitFormBoundaryiKeP7DQBTsft34cT\r\nContent-Disposition: form-data; name=\"text\"\r\n\r\n![github-macros-shipit](https://camo.githubusercontent.com/17d003fb9b66b81d64d7a25942b0c747be31f157560b322ede8666f51661a858/68747470733a2f2f6d65646961322e67697068792e636f6d2f6d656469612f764d4e6f4b4b7a6e4f72554a692f67697068792e6769663f6369643d373930623736313136326533613935316636343130613936613937333436613031626564373330393639656465636463267269643d67697068792e6769662663743d67)\r\n------WebKitFormBoundaryiKeP7DQBTsft34cT\r\nContent-Disposition: form-data; name=\"authenticity_token\"\r\n\r\nxoRGdHvHRV7g/n5AuFSX+xpljd5BS43HluJRVGcK4c02kyCKi8nZgcm9ffJPsaNV23iaOEnfy6bJihBfXoA27w==\r\n------WebKitFormBoundaryiKeP7DQBTsft34cT--\r\n",
  "method": "POST",
});

3. process the response and extract the URL

At "add" cloud function

1. if the provided URL is github's, do the basic validations (name not taken, url is valid, file type is valid)
   then insert it to the DB and return a partial response to the client. Without thumbnail or gif_thumbnail
   then, trigger another cloud function that will do all the post processing of recreating the real
   metadata

2. if the provided URL is not github's, the post processing should only create the new reports and usages intries




donations: https://carlosbecker.com/donate/
https://www.buymeacoffee.com/
*/