/*
    TODO
    1. basic add flow with some client side validation
    2. keep track of the last 100 usages, store it in local storage, load it during boot and place the top X 
       need to see how we make sure we will know how to load more suggestions nicely
       with top usages - DONE but needs to be tested
    3. settings - configure more sites and set the action keys
    4. fetch and show small footer from the server? Ask Sivan
    5. test load more

    6. Move to error codes from the server, interperate them here
    7. Add support for thumbnail + gifs thumbnails (add thumbnail, is_gif and gif_thumbnail)
        - We wanna show a small thumbnail instead of the real image
        - We wanna show a thumbnail with gif icon on top of it. When the user hovers
          We replace it with reduced gif
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

const numberOfTopUsagesToDisplay = 20;
const maxTopUsagesToStore = 100;
const maxSuggestionsFreshnessDuration = 60 * 60 * 24;
const macroNamePrefix = 'github-macros-';
const loadMorePixelsBeforeScrollEnd = 200;
const hostnameToPattern = new Map();
const contentCache = new Map();
const macroNameToUrl = new Map();
const tooltipsCache = new Map();
const targetIdToTarget = new Map();
const macroTopUsages = [];
const nameToTopUsage = new Map();

closeTooltipOnClickOutside = true;

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

    setTimeout(() => {
        updateTopUsages(macro);
        $.post( 
            "https://us-central1-github-macros.cloudfunctions.net/mutate/",
            { type: "use", name: macro["name"] },
        );
    }, 0);
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
            $.post( "https://us-central1-github-macros.cloudfunctions.net/mutate/", { type: "report", name: item["name"] } );
        };
        divs[i % 2].appendChild(image);
    }
}

createTooltip = function(targetId, target) {
    const idPrefix = targetId + '_';
    const magGlassSrc = chrome.runtime.getURL('img/icons/mglass-4x.png');
    const plusSrc = chrome.runtime.getURL('img/icons/plus-4x.png');
    const errorXSrc = chrome.runtime.getURL('img/icons/x-4x.png');
    const successVSrc = chrome.runtime.getURL('img/icons/v-4x.png');
    
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
            onOpenComplete: function () {
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
            onClose: function () {
                if (resizeObserver) {
                    resizeObserver.disconnect();
                }
                handleCloseTooltip(targetId);
            },
            content:`
            <div id="${idPrefix}macroMainWindow" style="width: 100%; height: 100%; position: absolute; top: 0px; left: 0px; right: 0px; bottom: 0px; overflow: hidden">
                <div style="display: flex; flex-direction: row; height: 55px;">
                    <div style="display: flex; flex-direction: row; width: 100%; height: auto; margin: 10px; border-width: 1px; border-style: solid none solid solid; border-color: #234C87; border-radius: 18px; overflow: auto">
                        <div style="display: flex; margin-left: 10px; width: 15px; height: 15px; justify-content: center; align-items: center; height: 100%; ">
                            <img id="${idPrefix}searchIcon" style="width: 15px; height: 15px;" src="${magGlassSrc}" />
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
                    <img style="width: 7px; height: 7px" src="${plusSrc}" />
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
                                    <img src="${errorXSrc}" style="width: 12px; height: 12px"/>
                                    <div id="${idPrefix}addNewMacroErrorMessage" class="gh-macros-add-new-error-message">
                                    </div>
                                </div>
                            </div>
                        </div> 
                        <div id="${idPrefix}addNewMacroSuccess" style="display: none;flex-direction: column;width: 100%;height: 100%;align-items: center;justify-content: center;">
                            <img src="${successVSrc}" style="width: 34px;height: 25px;">
                            <text style="font-family: 'Pragati Narrow';font-weight: 400;font-size: 16px;color: white;user-select: none;margin: 20;text-align: center;">It was added succesfully to the pile and you can start using it! Thanks for contributing :)</text>
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

findClosestButton = function(target, buttonText) {
    const pathFromTargetToRoot = new Map();

    var cur = target;
    var dist = 0;
    while (cur) {
        pathFromTargetToRoot.set(cur, dist);
        dist++;
        cur = cur.parentNode;
    }

    var minDist = Number.MAX_SAFE_INTEGER;
    var closestButton;
    var commonAncestor;

    $('button').each(function() {
        if (this.innerText !== buttonText) {
            return
        }

        var curDist = 0;
        var curNode = this;

        while (!pathFromTargetToRoot.has(curNode)) {
            curDist++;
            curNode = curNode.parentNode;
        }

        const totalDist = curDist + pathFromTargetToRoot.get(curNode);
        if (totalDist < minDist) {
            minDist = totalDist;
            closestButton = this;
            commonAncestor = curNode;
        }
      });

      return  {
          'button': closestButton,
          'common_ancestor': commonAncestor,
      }
}

createTooltipMeta = function(target) {
    const targetId = makeid(10)
    const tooltip = createTooltip(targetId, target);

    writeButtonMetadata = findClosestButton(target, 'Write')
    previewButtonMetadata = findClosestButton(target, 'Preview')

    const newTooltipMeta = {
        'target_id': targetId,
        'tooltip': tooltip,
        'write_button': writeButtonMetadata['button'],
        'preview_button': previewButtonMetadata['button'],
        'common_ancestor': previewButtonMetadata['common_ancestor'],
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
    const searchInput = getElement(targetId, 'macroSearchInput');

    macrosSection.addEventListener('scroll', function(e) {
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
                    ongoingRequests.delete(searchText);
                    // check if in the mean time, the user changed the input
                    if (searchText !== searchInput.value) {
                        return;
                    }

                    updateUIWithContent(targetId, content, true);
                },
                () => { loadingMore = false; },
            )
        }
      });
}

/*
let addSelfDestructingEventListener = (element, eventType, callback) => {
    let handler = () => {
        callback();
        element.removeEventListener(eventType, handler);
    };
    element.addEventListener(eventType, handler);
};
*/
initCloseTooltipLogic = function(targetId) {
    const mainMacroWindow = getElement(targetId, 'macroMainWindow')

    document.addEventListener('click', (event) => {
        const withinBoundaries = event.composedPath().includes(mainMacroWindow)
        if (!withinBoundaries && closeTooltipOnClickOutside) {
            tooltipsCache.get(targetIdToTarget.get(targetId))['tooltip'].close();
        } 
    })
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
    openMacroCreationButton.onmouseover = function() {openMacroCreationButton.style.background='#526683'};
    openMacroCreationButton.onmouseout = function() {openMacroCreationButton.style.background='#234C87'};
    openMacroCreationButton.onclick = function() {
        // $.ajax({
        //     url: 'https://api.resmush.it/ws.php?img=https://user-images.githubusercontent.com/10358078/142379224-23b6e6e5-d45d-4bc6-a183-733b831a622d.jpeg',
        //     success: function(responseText) {
        //         console.log(responseText);
        //     },
        //     fail: function() {
        //         console.log("failed to send request");
        //     }
        // });
        showAddMacroUI(targetId)
    };

    // set up add new macro UI
    getElement(targetId, 'addNewMacroCloseMargins').onclick = () => {hideAddMacroUI(targetId);}
    getElement(targetId, 'addNewMacroCloseButton').onclick = () => {hideAddMacroUI(targetId);}

    getElement(targetId, 'addNewMacroButton').onclick = () => {addNewMacro(targetId);}
    
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
            return `Image exceeds 1.5Mb. Please reduce its size and try again. You can use <a target="_blank" href="https://ezgif.com/optimize">this</a> website to do it`
        case ErrorCodes.FileFormatNotSupported:
            return "File format is not supported"
        case ErrorCodes.TransientError:
            return "Something went wrong, please try again"
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

var addingNewMacro = false

// isGitHubMediaLink = function(macroURL) {
//     try {
//         url = new URL(macroURL);
//         return url.hostname.endsWith('githubusercontent.com');
//     } catch {
//         return false;
//     }
// }

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


    addingNewMacro = true;

    $.ajax({
        url: `https://us-central1-github-macros.cloudfunctions.net/preprocess/?name=${macroName}&url=${macroURL}`,
        success: function(responseText) {
            console.log(responseText)
        },
        error: function() {
            addingNewMacro = false;
            addNewMacroShowErrorMessage(targetId, ErrorCodes.TransientError); 
        }
    });




    // if (isGitHubMediaLink(macroURL)) {
    //     uploadNewMacro(targetId, macroName, macroURL, true);
    // } else {
    //     createGitHubLink(
    //         targetId,
    //         macroURL,
    //         (githubMediaURL) => {
    //             uploadNewMacro(targetId, macroName, githubMediaURL, false);
    //         },
    //         () => {
    //             addingNewMacro = false;
    //             addNewMacroShowErrorMessage(targetId, ErrorCodes.URLHostnameNotSupported);
    //         },
    //     )
    // }

    


    

    

    

    /*
     * 1. inject the URL
     * 2. trigger preview by sending this event to the Preview button:
     *  we can query all buttons with Preview text in it and send this event to the button
     *  const mouseoverEvent = new Event('mouseenter');
     *  previewButton.dispatchEvent(mouseoverEvent);
     * 3. search the dom for the new image
     * 4. extract the URL
     * 5. remove the injected URL + restore mouse cursor
     * 6. In case of a failure, let the user know they will have to do it manually
     */

    /*
     * proposal for supporting thumbnails
     * after adding the initial image, load it in JS and, reduce its size and then
     * use the same tecnique to create another github image. Same for Gifs where we want
     * to have an image instead with icon of "gif" on top of it.
     * https://stackoverflow.com/questions/42092640/javascript-how-to-reduce-image-to-specific-file-size
     * 
     * reduce size of images: https://jsfiddle.net/qnhmytk4/3/
     * gid image extraction: https://jsfiddle.net/7gtLtkbw/
     */
    
}

createGitHubLink = function(targetId, macroURL, successCallback, failureCallback) {
    target = targetIdToTarget.get(targetId);
    metadata = tooltipsCache.get(target)

    if (!metadata['preview_button'] || !metadata['write_button']) {
        failureCallback()
        return;
    }

    targetText = target.value;
    targetSelectionStart = target.selectionStart;

    const macroToInject = getInjectedMacro('gh-macros-new-macro', macroURL);
    const newValue = target.value.slice(0, target.selectionStart - 1) + macroToInject + target.value.slice(target.selectionStart);
    target.value = newValue;

    var observer;
    var imageSearchInterval;

    const callback = function(mutationsList, observer) {
        const images = document.getElementsByTagName('img');
        for (i=0; i<images.length; i++) {
            const image = images[i];
            if (image.alt === 'github-macros-gh-macros-new-macro') {
                target.value = targetText;
                target.selectionStart = targetSelectionStart;
                target.selectionEnd = targetSelectionStart;
                clearInterval(imageSearchInterval);
                observer.disconnect();
                successCallback(image.src)
            }
        }
    }

    observer = new MutationObserver(callback);

    // Start observing the target node for configured mutations
    observer.observe(document.body/*metadata['common_ancestor']*/, { attributes: true, childList: true, subtree: true });

    setTimeout(
        () => {
            var times = 0
            imageSearchInterval = setInterval(
                () => {
                    if (times > 10) {
                        clearInterval(imageSearchInterval);
                        target.value = targetText;
                        target.selectionStart = targetSelectionStart;
                        target.selectionEnd = targetSelectionStart;
                        failureCallback();
                        return;
                    }

                    closeTooltipOnClickOutside = false;
                    metadata["preview_button"].click();
                    metadata["write_button"].click();
                    closeTooltipOnClickOutside = true;

                    // const images = document.getElementsByTagName('img');
                    // for (i=0; i<images.length; i++) {
                    //     const image = images[i];
                    //     if (image.alt === 'github-macros-gh-macros-new-macro') {
                    //         target.value = targetText;
                    //         target.selectionStart = targetSelectionStart;
                    //         target.selectionEnd = targetSelectionStart;
                    //         clearInterval(imageSearch);
                    //         successCallback(image.src); 
                    //         return;
                    //     }
                    //     console.log(image.alt)
                    // }

                    // $(metadata['common_ancestor']).children('img').each(function() {
                    //     if (this.alt === 'github-macros-gh-macros-new-macro') {
                    //         target.value = targetText;
                    //         target.selectionStart = targetSelectionStart;
                    //         target.selectionEnd = targetSelectionStart;
                    //         clearInterval(imageSearch);
                    //         successCallback(this.src); 
                    //         return;
                    //     }
                    //     console.log(this.alt)
                    // })

                    times++;
                },
                200
            )
        },
        1,
    )

    var triggerUIChange;

    // const callback = function(mutationsList, observer) {
    //     console.log(mutationsList)
    //     $(metadata['common_ancestor']).children('img').each(function() {
    //         console.log(this.alt)
    //     })
    // }

    // const observer = new MutationObserver(callback);

    // // Start observing the target node for configured mutations
    // observer.observe(metadata['common_ancestor'], {attributes: true});

    // var times = 0
    // triggerUIChange = setInterval(
    //     () => {
    //         metadata["preview_button"].click()
    //         metadata["write_button"].click()

    //         // exit in case times > X
    //     },
    //     1000
    // )

    // Later, you can stop observing
    //observer.disconnect();

    // we need to get the write tab button as well
    // we need to detect click outside ourselfs. Usually we will close
    // but in this specific case, we shouldn't
    // we need to click on Preview and then on Write back so the image
    // will be placed.
    // we need to repeat it several times since it might take
    // some time for github to respond with the preview

    // initHideWindow = function() {
    //     const target = document.getElementById('macroMainWindow')

    //     document.addEventListener('click', (event) => {
    //         const withinBoundaries = event.composedPath().includes(target)

    //         if (withinBoundaries) {
    //             console.log('Click happened inside element')
    //         } else {
    //             console.log('Click happened **OUTSIDE** element')
    //         } 
    //     })
    // }
}

uploadNewMacro = function(targetId, macroName, macroURL, revalidateURL) {
    uploadNewMacroFunc = () => {
        $.post(
            "https://us-central1-github-macros.cloudfunctions.net/mutate/",
            { type: "add", name: macroName, url: macroURL },
            (responseText) => {
                const response = JSON.parse(responseText);
                if (response['code'] != ErrorCodes.Success) {
                    addNewMacroShowErrorMessage(targetId, response['code']); 
                    return
                }

                addNewMacroShowSuccessMessage(targetId);

                macroNameToUrl[macroName] = macroURL
                contentCache.forEach(
                    (key, value) => {
                        if (macroName.includes(key)) {
                            value['data'].unshift({'name': macroName, 'url': macroURL})
                        }
                    },
                )
            },
        ).fail(
            () => {
                addNewMacroShowErrorMessage(targetId, ErrorCodes.TransientError); 
            },
        ).always(() => {addingNewMacro = false;});    
    }

    if (!revalidateURL) {
        uploadNewMacroFunc();
        return
    }

    const img = document.createElement('img')
    img.onload = uploadNewMacroFunc;
    img.onerror = () => {
        addNewMacroShowErrorMessage(targetId, ErrorCodes.InvalidURL);
        addingNewMacro = false;
    }
    img.src = macroURL;
}

initNewTooltip = function(targetId) {
    initSearchInput(targetId);
    initMacrosScrollLogic(targetId);
    initCloseTooltipLogic(targetId);
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

updateTopUsages = function(macro) {
    const macroName = macro['name'];
    if (nameToTopUsage.has(macroName)) {
        topUsage = nameToTopUsage.get(macroName);
        topUsage['usages'] = topUsage['usages'] + 1;
        macroTopUsages.sort(macroTopUsages, (a,b) => {return a['usages'] > b['usages']});
    } else if (nameToTopUsage.size == maxTopUsagesToStore) {
        topUsageToRemove = macroTopUsages.pop();
        nameToTopUsage.delete(topUsageToRemove['name'])
    }

    if (!nameToTopUsage.has(macroName)) {
        const newTopUsage = {
            'name': macroName,
            'url': macro['url'],
            'usages': 1,
        }
        macroTopUsages.push(newTopUsage);
        nameToTopUsage.set(macroName, newTopUsage);
    }

    chrome.storage.sync.set({'top_usages': JSON.stringify(macroTopUsages)});
}

loadSuggestionsFromStorage = function() {
    chrome.storage.sync.get(['suggestions', 'suggestions_freshness'], function(items) {
        if ('top_usages' in items) {
            macroTopUsages = JSON.parse(items['top_usages'])
            macroTopUsages.sort(macroTopUsages, (a,b) => {return a['usages'] > b['usages']})
            for (var i = 0; i < macroTopUsages.length; i++) { 
                const topUsage = macroTopUsages[i];
                nameToTopUsage.set(topUsage['name'], topUsage);
            }

            updateCacheWithNewContent('', macroTopUsages.slice(0, numberOfTopUsagesToDisplay), false, true)
        }
        
        if (!('suggestions' in items) || !('suggestions_freshness' in items)) {
            return;
        }

        if (Date.now() - parseInt(items['suggestions_freshness']) > maxSuggestionsFreshnessDuration) {
            return;
        }

        if ('suggestions' in items) {
            content = JSON.parse(items['suggestions'])
            updateCacheWithNewContent('', content['data'], true, content['has_more']);
        }
    });
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

window.onload = function() {
    initSupportedHostNames();

    if (!hostnameToPattern.has(location.hostname)) {
        return;
    }

    initKeyboardListeners();
    loadSuggestionsFromStorage()
    processGithubMacroImages();
}