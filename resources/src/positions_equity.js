import Chart, { barChartOptions, barStyle, gainColor, lossColor } from './charts';
import ChartDataLabels from 'chartjs-plugin-datalabels';
import { chartColor, pageData } from './common';

function share(ctx) {
    const total = ctx.dataset.data.reduce((sum, v) => sum + v, 0);
    return ctx.dataset.data[ctx.dataIndex] / total * 100;
}

function drawDoughnutChart(chartData) {
    const values = chartData.valueMicros.map(v => Math.round(v / 1e6));
    new Chart(document.getElementById('equity-canvas'), {
        type: 'doughnut',
        data: {
            labels: chartData.assetNames,
            datasets: [{ data: values, backgroundColor: values.map((_, i) => chartColor(i)) }],
        },
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

function drawBarChart(canvasId, labels, valueMicros, color, currency) {
    const values = valueMicros.map(v => v / 1e6);
    new Chart(document.getElementById(canvasId), {
        type: 'bar',
        data: { labels, datasets: [{ ...barStyle, data: values, backgroundColor: values.map(color) }] },
        options: barChartOptions(currency),
    });
}

export function init() {
    drawDoughnutChart(pageData('equity-chart-data'));
    const data = pageData('quarterly-chart-data');
    if (!data) return;
    // Sections without data have no chart.
    if (document.getElementById('quarterly-purchases-canvas')) {
        drawBarChart('quarterly-purchases-canvas', data.labels, data.purchases, () => chartColor(0), data.currency);
    }
    if (document.getElementById('quarterly-pl-canvas')) {
        drawBarChart('quarterly-pl-canvas', data.labels, data.profitLoss, v => (v < 0 ? lossColor : gainColor), data.currency);
    }
}
