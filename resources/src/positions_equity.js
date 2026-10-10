import Chart from 'chart.js/auto';
import ChartDataLabels from 'chartjs-plugin-datalabels';
import { chartColor, loadPref, savePref } from './common';

const hiddenKey = 'kontoo.positions.equityChartHidden';
let chart = null;

function share(ctx) {
    const total = ctx.dataset.data.reduce((sum, v) => sum + v, 0);
    return ctx.dataset.data[ctx.dataIndex] / total * 100;
}

function drawDoughnutChart(chartData) {
    const values = chartData.valueMicros.map(v => Math.round(v / 1e6));
    if (!chart) {
        chart = new Chart(document.getElementById('equity-canvas'), {
            type: 'doughnut',
            data: {},
            plugins: [ChartDataLabels],
            options: {
                animation: false,
                maintainAspectRatio: false,
                plugins: {
                    legend: { position: 'right' },
                    tooltip: {
                        callbacks: {
                            label: ctx => `${ctx.formattedValue} ${chartData.currency} (${share(ctx).toFixed(1)}%)`,
                        },
                    },
                    // Shares on the slices; slices too small for text only have the tooltip.
                    datalabels: {
                        display: ctx => share(ctx) >= 4,
                        formatter: (value, ctx) => `${share(ctx).toFixed(0)}%`,
                        color: 'white',
                        font: { weight: 600 },
                    },
                },
            },
        });
    }
    chart.data = {
        labels: chartData.assetNames,
        datasets: [{ data: values, backgroundColor: values.map((_, i) => chartColor(i)) }],
    };
    chart.update('none');
}

async function fetchAndDrawEquity() {
    try {
        const dateParam = new URLSearchParams(window.location.search).get('date');
        const endTimestamp = dateParam ? new Date(dateParam).getTime() : Date.now();
        const resp = await fetch('/kontoo/charts/equity', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ endTimestamp }),
        });
        if (!resp.ok) throw new Error(`Server returned status ${resp.status}`);
        const result = await resp.json();
        if (result.status !== 'OK') throw new Error(result.error);
        drawDoughnutChart(result);
    } catch (error) {
        console.error('Error fetching equity:', error);
    }
}

// Whether the chart is hidden is remembered per browser, like the other position charts.
function render() {
    const hidden = loadPref(hiddenKey) === 'true';
    document.getElementById('equity-chart').hidden = hidden;
    document.getElementById('equity-chart-show').hidden = !hidden;
    if (!hidden) fetchAndDrawEquity();
}

function setHidden(hidden) {
    savePref(hiddenKey, String(hidden));
    render();
}

export function init() {
    document.getElementById('equity-chart-hide').addEventListener('click', () => setHidden(true));
    document.getElementById('equity-chart-show').addEventListener('click', () => setHidden(false));
    render();
}
