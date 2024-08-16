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
        const statusDiv = document.getElementById("status-callout");
        if (statusDiv) {
            if (data.status !== "OK") {
                statusDiv.textContent = data.error;
                statusDiv.className = "callout-err";
            } else {
                statusDiv.textContent = `Added ${data.itemsImported} quotes and/or exchange rates.`;
                statusDiv.className = "callout-ok";
            }    
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
