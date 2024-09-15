// Shared functions used by one or more HTML templates and snippets.

export function hideCallout(elementId = "status-callout") {
    const div = document.getElementById(elementId);
    if (!div) {
        console.log(`No callout element with ID ${elementId}`);
        return;
    }
    div.classList.add("hidden");
}
export function callout(text, className = "callout-ok", elementId = "status-callout") {
    const div = document.getElementById(elementId);
    if (!div) {
        console.log(`No callout element with ID ${elementId}`);
        return;
    }
    div.textContent = text;
    div.classList.remove("hidden", "callout-ok", "callout-err", "callout-warn");
    div.classList.add(className);
}
export function calloutError(text, elementId) {
    callout(text, "callout-err", elementId);
}
export function calloutWarning(text, elementId) {
    callout(text, "callout-warn", elementId);
}
export function calloutStatus(status, text, elementId) {
    let className = "callout-err";
    if (status === "OK") {
        className = "callout-ok";
    } else if (status === "PARTIAL_SUCCESS") {
        className = "callout-warn";
    }
    callout(text, className, elementId);
}

// Used as part of registerQuotesSubmit() below.
export async function handleQuotesSubmit(e) {
    const inputs = document.querySelectorAll("input.selector:checked");
    const request = {
        quotes: [],
        exchangeRates: [],
    };
    inputs.forEach(inp => {
        if (inp.name === "quote") {
            request.quotes.push({
                assetID: inp.dataset.asset,
                date: inp.dataset.date,
                priceMicros: parseInt(inp.dataset.price)
            });
        } else if (inp.name === "exchangerate") {
            request.exchangeRates.push({
                baseCurrency: inp.dataset.basecurrency,
                quoteCurrency: inp.dataset.quotecurrency,
                date: inp.dataset.date,
                priceMicros: parseInt(inp.dataset.price)
            });
        }
    });
    try {
        const response = await fetch("/kontoo/quotes", {
            method: "POST",
            body: JSON.stringify(request),
            headers: {
                "Content-Type": "application/json"
            }
        });
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const data = await response.json();
        if (data.status === "OK") {
            callout(`Added ${data.itemsImported} quotes and/or exchange rates.`);
            inputs.forEach(inp => inp.checked = false);
        } else {
            calloutStatus(data.status, data.error);
        }
    }
    catch (error) {
        console.error("Error on submit:", error);
    }
}

// Registers event listeners for quotes upload functionality:
//
// Expects the following elements to be present in the DOM:
// * <checkbox id="select-all">
// * <input type="checkbox" class="selector" name="quote|exchangerate" data-*>
//   * These are the data-carrying input checkboxes from which the request JSON is built.
// * <button id="submit">
// * (optional) <div id="status-callout"> to display success/error messages in.
export function registerQuotesSubmit() {
    const selectAll = document.getElementById("select-all");
    if (selectAll) {
        selectAll.addEventListener("change", function (e) {
            // Get state of clicked selectAll here, to set inputs all to this value.
            const isChecked = e.target.checked;
            const inputs = document.querySelectorAll("input.selector");
            inputs.forEach(inp => inp.checked = isChecked);
        });
    }
    const submit = document.getElementById("submit");
    if (submit) {
        submit.addEventListener("click", handleQuotesSubmit);
    }
}

// Registers a dropdown widget. callback will be called with the
// clicked option div as the only argument.
export function registerDropdown(id, callback) {
    const dropdown = document.getElementById(id);
    if (!dropdown) {
        console.error("Trying to register a dropdown that does not exist:", id);
        return;
    }
    const button = dropdown.querySelector(".combo-button");
    // Make the dropdown focusable with tab.
    dropdown.setAttribute("tabindex", "0");
    // Open/close the dropdown when gaining/losing focus.
    dropdown.addEventListener("focus", function () {
        dropdown.classList.add("open");
    });
    dropdown.addEventListener("blur", function () {
        dropdown.classList.remove("open");
    });
    // Toggle the dropdown menu when clicking the button.
    // Use "mousedown" instead of "click" to handle the event
    // before the dropdown loses its focus.
    button.addEventListener("mousedown", function (event) {
        // Prevent dropdown from losing focus:
        event.preventDefault();
        if (document.activeElement === dropdown) {
            dropdown.blur();
        } else {
            dropdown.focus();
        }
    });
    // Set the dropdown value on select and hide the dropdown options.
    dropdown.addEventListener("click", (event) => {
        const option = event.target;
        if (!option.classList.contains("combo-option")) {
            return;
        }
        button.textContent = option.textContent;
        button.dataset.value = option.dataset.value;
        dropdown.classList.remove("open");
        callback(option);
    });
}

export function registerContextMenu(element, callback) {
    if (!element.classList.contains("contextmenu")) {
        console.error("Context menu must have class contextmenu");
        return;
    }
    const menu = element.querySelector(".contextmenu-options");
    if (!menu) {
        console.error("Cannot register context for element: no .contextmenu child div");
        return;
    }
    element.setAttribute("tabindex", "0");
    element.addEventListener("focus", function () {
        element.classList.add("open");
    });
    element.addEventListener("blur", function () {
        element.classList.remove("open");
    });
    menu.addEventListener("click", (event) => {
        const option = event.target;
        if (!option.classList.contains("contextmenu-option")) {
            return;
        }
        element.blur();
        callback(option);
    })
}
