import { registerDropdown, registerContextMenu } from "./common";
import Chart from 'chart.js/auto';
import 'chartjs-adapter-date-fns';


let chart = null;

function followUrl(item) {
    window.location.href = item.dataset.url;
}

function contextMenuSelected(item) {
    const action = item.dataset.action;
    if (["add-entry", "show-ledger", "edit-asset"].includes(action)) {
        return followUrl(item);
    }
    console.error(`Unhandled action in context menu: ${action}`);
}

function drawDoughnutChart(chartData) {
    if (!chart) {
        chart = new Chart(
            document.getElementById('equity-canvas'),
            {
                type: 'doughnut',
                data: {},
                options: {
                    animation: false,
                    plugins: {
                        legend: {
                            position: 'bottom',
                            display: true
                        },
                        title: {
                            display: true,
                            text: `Equity asset allocation (${chartData.currency})`
                        },
                        tooltip: {
                            callbacks: {
                                label: function (ctx) {
                                    // Display both absolute and % values in tooltip.
                                    if (ctx.dataset.label) {
                                        const pct = ctx.dataset.data[ctx.dataIndex] / ctx.dataset.data.reduce((s, r) => (s + r), 0) * 100;
                                        const formattedValue = ctx.formattedValue + " (" + pct.toFixed(1) + "%)";
                                        return ctx.dataset.label + ": " + formattedValue;
                                    }
                                    return undefined;
                                }
                            }
                        }
                    }
                },
            }
        );
    }
    chart.data = {
        labels: chartData.assetNames,
        datasets: [{
            label: "Mkt value",
            data: chartData.valueMicros.map(v => Math.round(v / 1e6))
        }]
    }
    chart.update('none');
}

async function fetchAndDrawEquity() {
    try {
        const dateParam = new URLSearchParams(window.location.search).get("date");
        const endTimestamp = dateParam ? new Date(dateParam).getTime() : Date.now();
        const resp = await fetch("/kontoo/charts/equity", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                "endTimestamp": endTimestamp,
            })
        });
        if (!resp.ok) {
            throw new Error(`Server returned status ${resp.status}`);
        }
        const result = await resp.json();
        if (result.status !== "OK") {
            console.log("Response not OK:", result);
            return;
        }
        drawDoughnutChart(result);
        document.getElementById("equity-chart").classList.remove("hidden");
    }
    catch (error) {
        console.error("Error fetching equity:", error);
        return;
    }
}

export function init() {
    registerDropdown("months-dropdown", followUrl);
    registerDropdown("years-dropdown", followUrl);
    document.querySelectorAll(".contextmenu.entry-actions").forEach((td) => {
        registerContextMenu(td, contextMenuSelected);
    });
    const chartDiv = document.querySelector("#equity-chart");
    chartDiv.querySelector(".close").addEventListener("click", () => {
        chartDiv.classList.add("hidden");
    });
    fetchAndDrawEquity();
}
