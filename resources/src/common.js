// Shared functions used by one or more HTML templates and snippets.

// Returns the data the server embedded in the page as
// <script id="<id>" type="application/json">, or null if there is none.
export function pageData(id) {
    const script = document.getElementById(id);
    return script ? JSON.parse(script.textContent) : null;
}

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

// Colors of chart series (lines, donut segments), in order of use.
const chartColors = ['#2457a7', '#d9822b', '#2f9e6e', '#b83280', '#7c5cc4', '#0f8a9d', '#a16207', '#64748b',
    '#5b8fd6', '#e0a458', '#6cbf8f', '#d77fb3'];
export function chartColor(i) {
    return chartColors[i % chartColors.length];
}

// Per-browser preferences. Storage may be unavailable (e.g. blocked site data):
// then nothing is remembered and the defaults apply.
export function loadPref(key) {
    try {
        return localStorage.getItem(key);
    } catch {
        return null;
    }
}
export function savePref(key, value) {
    try {
        localStorage.setItem(key, value);
    } catch {
        // Not remembered.
    }
}

// Saving a form is always an explicit button press: Enter in a field never submits.
export function preventEnterSubmit(form) {
    form.addEventListener("keydown", event => {
        if (event.key === "Enter" && event.target instanceof HTMLInputElement) event.preventDefault();
    });
}

// Used as part of registerQuotesSubmit() below.
async function handleQuotesSubmit() {
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
// * <input type="checkbox" class="selector" name="quote|exchangerate" data-*>
//   * These are the data-carrying input checkboxes from which the request JSON is built.
// * (optional) <input type="checkbox" class="select-all"> in a table header,
//   to select or clear all rows of that table.
// * <button id="submit">, labelled with the number of selected rows.
// * (optional) <div id="status-callout"> to display success/error messages in.
export function registerQuotesSubmit() {
    const submit = document.getElementById("submit");
    if (!submit) {
        return;
    }
    const update = () => {
        for (const selectAll of document.querySelectorAll("input.select-all")) {
            const rows = [...selectAll.closest("table").querySelectorAll("input.selector")];
            const checked = rows.filter(row => row.checked).length;
            selectAll.checked = checked > 0 && checked === rows.length;
            selectAll.indeterminate = checked > 0 && checked < rows.length;
        }
        const n = document.querySelectorAll("input.selector:checked").length;
        submit.textContent = `Import ${n} ${n === 1 ? "price" : "prices"}`;
        submit.disabled = n === 0;
    };
    document.querySelectorAll("input.select-all").forEach(selectAll => {
        selectAll.addEventListener("change", () => {
            selectAll.closest("table").querySelectorAll("input.selector").forEach(row => { row.checked = selectAll.checked; });
            update();
        });
    });
    document.querySelectorAll("input.selector").forEach(row => row.addEventListener("change", update));
    submit.addEventListener("click", async () => {
        submit.disabled = true; // Until update() below: prevents importing twice.
        await handleQuotesSubmit();
        update();
    });
    update();
}
