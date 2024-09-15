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

export function init() {
    // Per-row buttons
    document.getElementById("toggle-row-actions").addEventListener("click", () => {
        document.querySelectorAll(".action-column").forEach(elem => {
            elem.classList.toggle("hidden");
        });
    });

    document.getElementById("reload-ledger").addEventListener("click", reloadLedger);
    
    document.querySelectorAll("button.delete").forEach(button => {
        button.addEventListener("click", () => deleteLedgerEntry(parseInt(button.dataset.seq)));
    });
    
    // Filter query
    const filter = document.getElementById("filter");
    filter.addEventListener('keydown', async function (event) {
        if (event.key !== 'Enter') {
            return;
        }
        event.preventDefault();
        const query = event.target.value;
        try {
            const url = new URL(window.location.href);
            url.searchParams.set('q', query);
            url.searchParams.set('snippet', 'true')
            const response = await fetch(url);
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const html = await response.text();
            document.getElementById("ledger-table-div").innerHTML = html;
            url.searchParams.delete("snippet"); // Request the full page in the browser's history.
            history.pushState({ html: html, query: query }, "", url.href);
        }
        catch (error) {
            console.error("Error in query:", error);
        }
    });
    window.addEventListener("popstate", (event) => {
        if (event.state) {
            const state = event.state;
            document.getElementById("ledger-table-div").innerHTML = state.html;
            document.getElementById("filter").value = state.query;
        } else {
            // No state => just refresh the page
            location.reload();
        }
    });
}
