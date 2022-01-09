const button = document.getElementById("clearCacheButton");
button.onclick = function() {
  chrome.storage.sync.set({
    'suggestions': '',
    'suggestions_freshness': '',
    'system_message': '',
    'top_usages': '',
  });
}