import Chart, { barChartOptions, barStyle } from './charts';
import { chartColor, loadPref, savePref } from './common';

const hiddenKey = 'kontoo.positions.maturitiesChartHidden';
let chart = null;

function drawBarChart(maturities) {
    if (!chart) {
        // The tooltip shows each bucket's share of the total, too.
        const tooltipLabel = ctx => {
            const pct = ctx.raw / ctx.dataset.data.reduce((sum, v) => sum + v, 0) * 100;
            return `${ctx.formattedValue} ${maturities.currency} (${pct.toFixed(1)}%)`;
        };
        chart = new Chart(document.getElementById('maturities-canvas'), {
            type: 'bar',
            data: {},
            options: barChartOptions(maturities.currency, tooltipLabel),
        });
    }
    chart.data = {
        labels: maturities.bucketLabels,
        datasets: maturities.values.map(v => ({
            ...barStyle,
            backgroundColor: chartColor(0),
            data: v.valueMicros.map(x => Math.round(x / 1e6)),
        })),
    };
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
    }
    catch (error) {
        console.error("Error fetching maturities:", error);
        return;
    }
}

// Whether the chart is hidden is remembered per browser, like the chart on the "All" tab.
function render() {
    const hidden = loadPref(hiddenKey) === 'true';
    document.getElementById('maturities-chart').hidden = hidden;
    document.getElementById('maturities-chart-show').hidden = !hidden;
    if (!hidden) fetchAndDrawMaturities();
}

function setHidden(hidden) {
    savePref(hiddenKey, String(hidden));
    render();
}

export function init() {
    if (!document.getElementById('maturities-chart')) return; // Not shown when exchange rates are missing.
    document.getElementById('maturities-chart-hide').addEventListener('click', () => setHidden(true));
    document.getElementById('maturities-chart-show').addEventListener('click', () => setHidden(false));
    render();
}
