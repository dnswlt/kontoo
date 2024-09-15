import { calloutError, hideCallout } from "./common";

async function deleteLedgerEntry(sequenceNum) {
    try {
        const response = await fetch("/kontoo/entries/delete", {
            method: "POST",
            body: JSON.stringify({
                sequenceNum: sequenceNum
            }),
            headers: {
                "Content-Type": "application/json"
            }
        });
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const data = await response.json();
        if (data.status === "OK") {
            console.log(`Deleted ledger entry ${data.sequenceNum}.`);
            location.reload();
        } else {
            console.error("Could not delete ledger entry:", data.status, data.error);
        }
    }
    catch (error) {
        console.error("Error on submit:", error);
    }
}

async function reloadLedger() {
    try {
        const resp = await fetch("/kontoo/ledger/reload", {
            method: "POST"
        });
        if (!resp.ok) {
            throw new Error(`HTTP error: status ${resp.status}`);
        }
        location.reload();
    }
    catch (error) {
        console.error("Failed to reload:", error);
    }
}

function registerTableEventListeners() {
    document.querySelectorAll("button.delete").forEach(button => {
        button.addEventListener("click", () => deleteLedgerEntry(parseInt(button.dataset.seq)));
    });
}

async function filterEntries(query) {
    try {
        const url = new URL(window.location.href);
        url.searchParams.set('q', query);
        url.searchParams.set('snippet', 'true')
        const response = await fetch(url);
        if (!response.ok) {
            const body = await response.text();
            if (body) {
                throw new Error(body);
            }
            throw new Error(`Server responsed with status ${response.status}`);
        }
        // Hide any previously shown callout explicitly, since we don't reload the page.
        hideCallout();
        // Display result.
        const html = await response.text();
        document.getElementById("ledger-table-div").innerHTML = html;
        registerTableEventListeners();
        // Request the full page in the browser's history.
        url.searchParams.delete("snippet");
        history.pushState({ html: html, query: query }, "", url.href);
    }
    catch (error) {
        calloutError(error.message);
    }
}

export function init() {
    // Per-row buttons
    document.getElementById("toggle-row-actions").addEventListener("click", () => {
        document.querySelectorAll(".action-column").forEach(elem => {
            elem.classList.toggle("hidden");
        });
    });

    document.getElementById("reload-ledger").addEventListener("click", reloadLedger);

    registerTableEventListeners();

    // Filter query
    const filter = document.getElementById("filter");
    filter.addEventListener('keydown', async function (event) {
        if (event.key !== 'Enter') {
            return;
        }
        event.preventDefault();
        const query = event.target.value;
        await filterEntries(query);
    });
    window.addEventListener("popstate", (event) => {
        if (event.state) {
            const state = event.state;
            document.getElementById("ledger-table-div").innerHTML = state.html;
            document.getElementById("filter").value = state.query;
            registerTableEventListeners();
        } else {
            // No state => just refresh the page
            location.reload();
        }
    });
}
