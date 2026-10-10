import Chart from 'chart.js/auto';
import 'chartjs-adapter-date-fns';
import { format } from "date-fns";
import { enGB } from 'date-fns/locale';


let chart = null;

function drawBarChart(maturities) {
    if (!chart) {
        chart = new Chart(
            document.getElementById('maturities-canvas'),
            {
                type: 'bar',
                data: {},
                options: {
                    animation: false,
                    parsing: true,
                    scales: {
                        y: {
                            display: true,
                            title: {
                                display: true,
                                text: maturities.currency
                            }
                        }
                    },
                    plugins: {
                        legend: {
                            position: 'top',
                            display: true
                        },
                        title: {
                            display: true,
                            text: "Value by time to maturity (years)"
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
        labels: maturities.bucketLabels,
        datasets: maturities.values.map(v => ({
            label: v.label,
            data: v.valueMicros.map(x => Math.round(x / 1e6))
        }))
    }
    chart.update('none');
}

async function fetchAndDrawMaturities() {
    try {
        const dateParam = new URLSearchParams(window.location.search).get("date");
        const endTimestamp = dateParam ? new Date(dateParam).getTime() : Date.now();
        const resp = await fetch("/kontoo/positions/maturities", {
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
        drawBarChart(result.maturities);
        document.getElementById("maturities-chart").classList.remove("hidden");
    }
    catch (error) {
        console.error("Error fetching maturities:", error);
        return;
    }
}

export function init() {
    const chartDiv = document.querySelector("#maturities-chart");
    chartDiv.querySelector(".close").addEventListener("click", () => {
        chartDiv.classList.add("hidden");
    });
    fetchAndDrawMaturities();
}
