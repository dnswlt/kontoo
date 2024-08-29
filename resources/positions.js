import { registerDropdown, registerContextMenu } from "./common";
import Chart from 'chart.js/auto'
import 'chartjs-adapter-date-fns'
import { enGB } from 'date-fns/locale'

function followUrl(item) {
    window.location.href = item.dataset.url;
}

function toggleChartDisplay(assetId) {
    const url = new URL(window.location);
    const chart = url.searchParams.get("chart");
    let assetIds = chart ? chart.split(",") : [];
    if (assetIds.includes(assetId)) {
        assetIds = assetIds.filter(a => a !== assetId);
    } else {
        assetIds.push(assetId);
    }
    if (assetIds.length > 0) {
        url.searchParams.set("chart", assetIds.join(","));
    } else {
        url.searchParams.delete("chart");
    }
    window.history.pushState({}, '', url);
    fetchAndDrawTimelines();
}

function contextMenuSelected(item) {
    const action = item.dataset.action;
    if (["add-entry", "show-ledger"].includes(action)) {
        return followUrl(item);
    }
    if (action === "toggle-chart") {
        return toggleChartDisplay(item.dataset.id);
    }
    console.error(`Unhandled action in context menu: ${action}`);
}

let chart = null;

function drawTimelines(timelines) {
    if (!chart) {
        chart = new Chart(
            document.getElementById('chart'),
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
                                display: true,
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

function chartAssetIds() {
    const chart = new URLSearchParams(window.location.search).get("chart");
    if (!chart) {
        return [];
    }
    return chart.split(",");
}

async function fetchAndDrawTimelines() {
    const assetIds = chartAssetIds();
    if (assetIds.length == 0) {
        document.getElementById("chart-div").classList.add("hidden");
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
                "assetIds": assetIds,
                // Get everything up to and including selected date.
                "startTimestamp": 0,
                "endTimestamp": endTimestamp,
            })
        });
        const result = await resp.json();
        if (result.status !== "OK") {
            console.log("Response not OK:", result);
            return;
        }
        drawTimelines(result.timelines);
        document.getElementById("chart-div").classList.remove("hidden");
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
    document.querySelector("#chart-close-button").addEventListener("click", (event) => {
        document.getElementById("chart-div").classList.add("hidden");
        const url = new URL(window.location);
        url.searchParams.delete("chart");
        history.pushState({}, '', url);
    });
    window.addEventListener('popstate', () => {
        fetchAndDrawTimelines();
    })
    fetchAndDrawTimelines();
    history.replaceState({}, "", window.location.href);
}
