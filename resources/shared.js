// Shared functions used by one or more HTML templates and snippets.

function callout(text, className = "callout-ok", elementId = "status-callout") {
    const div = document.getElementById(elementId);
    if (!div) {
        console.log(`No callout element with ID ${elementId}`);
        return;
    }
    div.textContent = text;
    div.classList.remove("hidden", "callout-ok", "callout-err", "callout-warn");
    div.classList.add(className);
}
function calloutError(text, elementId) {
    callout(text, "callout-err", elementId);
}
function calloutWarning(text, elementId) {
    callout(text, "callout-warn", elementId);
}
function calloutStatus(status, text, elementId) {
    let className = "callout-err";
    if (status === "OK") {
        className = "callout-ok";
    } else if (status === "PARTIAL_SUCCESS") {
        className = "callout-warn";
    }
    callout(text, className, elementId);
}

// Used as part of registerQuotesSubmit() below.
async function handleQuotesSubmit(e) {
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
function registerQuotesSubmit() {
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
