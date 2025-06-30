import { registerDropdown, registerContextMenu, base64ToString, stringToBase64 } from "./common";
import Chart from 'chart.js/auto'
import 'chartjs-adapter-date-fns'
import { enGB } from 'date-fns/locale'


let chart = null;
let uiState = {
    assetIds: [],
    period: "1Y",  // Period to be displayed.
};

function followUrl(item) {
    window.location.href = item.dataset.url;
}

function contextMenuSelected(item) {
    const action = item.dataset.action;
    if (["add-entry", "update-balance", "show-ledger", "edit-asset"].includes(action)) {
        return followUrl(item);
    }
    if (action === "toggle-chart") {
        return toggleChartDisplay(item.dataset.id);
    }
    console.error(`Unhandled action in context menu: ${action}`);
}

function updateUIState(update) {
    uiState = { ...uiState, ...update }
    const url = new URL(window.location);
    url.searchParams.set("ui-state", stringToBase64(JSON.stringify(uiState)));
    window.history.pushState({}, '', url);
    fetchAndDrawTimelines();
}

function initUIState() {
    const stateParam = new URLSearchParams(window.location.search).get("ui-state");
    if (!stateParam) {
        return; // Nothing to do, use initial state.
    }
    try {
        uiState = JSON.parse(base64ToString(stateParam));
    } catch (error) {
        console.error("Invalid ui-state= param:", error);
    }
}

function toggleChartDisplay(assetId) {
    let assetIds = uiState.assetIds;
    if (assetIds.includes(assetId)) {
        assetIds = assetIds.filter(a => a !== assetId);
    } else {
        assetIds = assetIds.concat(assetId);
    }
    updateUIState({
        assetIds: assetIds
    });
}

function updateChartPeriod(period) {
    updateUIState({
        period: period
    });
}

function drawTimelines(timelines) {
    if (!chart) {
        chart = new Chart(
            document.getElementById('positions-canvas'),
            {
                type: 'line',
                options: {
                    animation: false,
                    parsing: false,
                    plugins: {
                        legend: {
                            display: true
                        }
                    },
                    scales: {
                        x: {
                            type: 'time',
                            adapters: {
                                date: {
                                    locale: enGB,
                                },
                            },
                            time: {
                                displayFormats: {
                                    day: 'd MMM',
                                    month: 'MMM yy'
                                },
                                tooltipFormat: 'd MMM yyyy'
                            }
                        },
                        y: {
                            min: 0,
                            title: {
                                display: false,
                                text: 'Value'
                            }
                        }
                    }
                },
                data: {
                    datasets: []
                }
            }
        );
    }
    chart.data.datasets = timelines.map(timeline => ({
        label: timeline.assetName,
        data: Array.from(timeline.timestamps.keys()).map(i => ({
            x: timeline.timestamps[i],
            y: timeline.valueMicros[i] / 1e6
        }))
    }));
    chart.update('none');
}

async function fetchAndDrawTimelines() {
    if (uiState.assetIds.length == 0) {
        const chart = document.getElementById("positions-chart");
        if (chart) {
            chart.classList.add("hidden");
        }
        return;
    }
    try {
        const dateParam = new URLSearchParams(window.location.search).get("date");
        const endTimestamp = dateParam ? new Date(dateParam).getTime() : Date.now();
        const resp = await fetch("/kontoo/positions/timeline", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                "assetIds": uiState.assetIds,
                "endTimestamp": endTimestamp,
                "period": uiState.period,
            })
        });
        const result = await resp.json();
        if (result.status !== "OK") {
            console.log("Response not OK:", result);
            return;
        }
        drawTimelines(result.timelines);
        document.getElementById("positions-chart").classList.remove("hidden");
    }
    catch (error) {
        console.error("Error fetching timeline:", error);
        return;
    }
}

export function init() {
    registerDropdown("months-dropdown", followUrl);
    registerDropdown("years-dropdown", followUrl);
    document.querySelectorAll(".contextmenu.entry-actions").forEach((td) => {
        registerContextMenu(td, contextMenuSelected);
    });
    const chartDiv = document.querySelector("#positions-chart");
    if (chartDiv) {
        chartDiv.querySelector(".close").addEventListener("click", () => {
            chartDiv.classList.add("hidden");
            const url = new URL(window.location);
            url.searchParams.delete("ui-state");
            history.pushState({}, '', url);
        });
        chartDiv.querySelectorAll(".chart-period button").forEach(button => {
            button.addEventListener("click", () => {
                chartDiv.querySelectorAll(".chart-period button").forEach(b => {
                    if (b === button) {
                        b.classList.add("selected");
                    } else {
                        b.classList.remove("selected");
                    }
                })
                updateChartPeriod(button.dataset.period);
            });
        });
    }
    window.addEventListener('popstate', () => {
        fetchAndDrawTimelines();
    })
    initUIState();
    fetchAndDrawTimelines();
    history.replaceState({}, "", window.location.href);
}
