import { registerDropdown, registerContextMenu } from "./common";
import Chart from 'chart.js/auto'
import 'chartjs-adapter-date-fns'
import { enGB } from 'date-fns/locale'

function followUrl(item) {
    window.location.href = item.dataset.url;
}

function drawTimeline(positionTimelineResponse) {
    if (positionTimelineResponse.status !== "OK") {
        console.log("Response not OK:", positionTimelineResponse);
        return;
    }

    new Chart(
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
                // labels: data.map(row => row.date),
                datasets:
                    positionTimelineResponse.timelines.map(timeline => ({
                        label: timeline.assetName,
                        data: Array.from(timeline.timestamps.keys()).map(i => ({
                            x: timeline.timestamps[i],
                            y: timeline.valueMicros[i] / 1e6
                        }))
                    }))
                // [
                //     {
                //         label: 'Value',
                //         // Data must be {x, y} for parsing: false to work (a performance optimization).
                //         data: data.map(row => { return { x: row.date.getTime(), y: row.value }; })
                //     }
                // ]
            }
        }
    );
}

async function fetchAndDrawTimelines() {
    try {
        const resp = await fetch("/kontoo/positions/timeline", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                // TODO: make asset IDs selectable in the UI
                "assetIds": [
                    "GOOG",
                ],
                // Get everything up to and including selected date.
                "startTimestamp": 0,
                "endTimestamp": new Date("2024-12-31").getTime()
            })
        });
        const result = await resp.json();
        drawTimeline(result);
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
        registerContextMenu(td, followUrl);
    });
    // TODO: remove
    fetchAndDrawTimelines();
}
