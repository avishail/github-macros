let changeColor = document.getElementById("changeColor");

chrome.storage.sync.get("color", ({ color }) => {
  changeColor.style.backgroundColor = color;
});

alert('boom!');

// When the button is clicked, inject setPageBackgroundColor into current page
changeColor.addEventListener("click", async () => {
    let [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  
    chrome.scripting.executeScript({
      target: { tabId: tab.id },
      function: setPageBackgroundColor,
    });
  });
  
  // The body of this function will be executed as a content script inside the
  // current page
  function setPageBackgroundColor() {
    if (maybeOpenTooltip) {
      maybeOpenTooltip()
    }
    // console.log(location.hostname)
    // console.log(document.activeElement)
    // console.log(openTooltip)
    // chrome.storage.sync.get("color", ({ color }) => {
    //   document.body.style.backgroundColor = color;
    // });
  }