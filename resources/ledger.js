export function init() {
    // Per-row buttons
    document.getElementById("toggle-row-actions").addEventListener("click", () => {
        document.querySelectorAll(".action-column").forEach(elem => {
            elem.classList.toggle("hidden");
        });
    });
    document.getElementById("reload-ledger").addEventListener("click", async function () {
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
    });
    document.querySelectorAll("button.delete").forEach(button => {
        button.addEventListener("click", async function () {
            try {
                const response = await fetch("/kontoo/entries/delete", {
                    method: "POST",
                    body: JSON.stringify({
                        sequenceNum: parseInt(button.dataset.seq)
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
        });
    });
    // Filter
    const filter = document.getElementById("filter");
    filter.addEventListener('keydown', async function (event) {
        if (event.key !== 'Enter') {
            return;
        }
        event.preventDefault();
        const query = event.target.value;
        try {
            const params = new URLSearchParams({
                snippet: true,
                q: query,
            });
            const response = await fetch(`/kontoo/ledger?${params}`);
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const html = await response.text();
            const div = document.getElementById("ledger-table-div");
            div.innerHTML = html;
            params.delete("snippet"); // Request the full page in the browser's history.
            history.pushState({ html: html, query: query }, "", `/kontoo/ledger?${params}`);
        }
        catch (error) {
            console.error("Error in query:", error);
        }
    });
    window.addEventListener("popstate", (event) => {
        if (event.state) {
            const state = event.state;
            const div = document.getElementById("ledger-table-div");
            div.innerHTML = state.html;
            const input = document.getElementById("filter");
            input.value = state.query;
        } else {
            // No state => just refresh the page
            location.reload();
        }
    });
}
