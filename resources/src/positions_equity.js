import Chart, { barChartOptions, barStyle } from './charts';
import ChartDataLabels from 'chartjs-plugin-datalabels';
import { chartColor, loadPref, savePref } from './common';

const hiddenKey = 'kontoo.positions.equityChartHidden';
const profitColor = '#2f9e6e';
const lossColor = 'rgb(219, 18, 34)'; // --red-number
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
                    legend: { position: 'right', labels: { usePointStyle: true, pointStyle: 'circle', boxWidth: 8, boxHeight: 8, padding: 10 } },
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

function drawBarChart(canvasId, labels, valueMicros, color, currency) {
    const values = valueMicros.map(v => v / 1e6);
    new Chart(document.getElementById(canvasId), {
        type: 'bar',
        data: { labels, datasets: [{ ...barStyle, data: values, backgroundColor: values.map(color) }] },
        options: barChartOptions(currency),
    });
}

// The quarterly charts sit below their tables; the button in the heading row
// hides or shows the chart, remembered per browser. Charts are drawn once visible.
function initQuarterlyChart(name, draw) {
    const chartDiv = document.getElementById(`${name}-chart`);
    if (!chartDiv) return; // Section without data.
    const button = document.querySelector(`[data-chart-toggle="${name}-chart"]`);
    const key = `kontoo.positions.${name}ChartHidden`;
    let drawn = false;
    const render = () => {
        const hidden = loadPref(key) === 'true';
        chartDiv.hidden = hidden;
        button.textContent = hidden ? 'Show chart' : 'Hide chart';
        if (!hidden && !drawn) {
            draw();
            drawn = true;
        }
    };
    button.addEventListener('click', () => {
        savePref(key, String(!chartDiv.hidden));
        render();
    });
    render();
}

export function init() {
    const quarterly = document.getElementById('quarterly-chart-data');
    if (quarterly) {
        const data = JSON.parse(quarterly.textContent);
        initQuarterlyChart('quarterly-purchases', () =>
            drawBarChart('quarterly-purchases-canvas', data.labels, data.purchases, () => chartColor(0), data.currency));
        initQuarterlyChart('quarterly-pl', () =>
            drawBarChart('quarterly-pl-canvas', data.labels, data.profitLoss, v => (v < 0 ? lossColor : profitColor), data.currency));
    }
    document.getElementById('equity-chart-hide').addEventListener('click', () => setHidden(true));
    document.getElementById('equity-chart-show').addEventListener('click', () => setHidden(false));
    render();
}
