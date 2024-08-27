export function init() {
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