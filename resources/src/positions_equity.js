import Chart, { barChartOptions, barStyle, gainColor, lossColor } from './charts';
import ChartDataLabels from 'chartjs-plugin-datalabels';
import { chartColor } from './common';

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

function drawBarChart(canvasId, labels, valueMicros, color, currency) {
    const values = valueMicros.map(v => v / 1e6);
    new Chart(document.getElementById(canvasId), {
        type: 'bar',
        data: { labels, datasets: [{ ...barStyle, data: values, backgroundColor: values.map(color) }] },
        options: barChartOptions(currency),
    });
}

export function init() {
    fetchAndDrawEquity();
    const quarterly = document.getElementById('quarterly-chart-data');
    if (!quarterly) return;
    // Sections without data have no chart.
    const data = JSON.parse(quarterly.textContent);
    if (document.getElementById('quarterly-purchases-canvas')) {
        drawBarChart('quarterly-purchases-canvas', data.labels, data.purchases, () => chartColor(0), data.currency);
    }
    if (document.getElementById('quarterly-pl-canvas')) {
        drawBarChart('quarterly-pl-canvas', data.labels, data.profitLoss, v => (v < 0 ? lossColor : gainColor), data.currency);
    }
}
