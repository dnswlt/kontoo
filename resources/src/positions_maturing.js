import Chart, { barChartOptions, barStyle } from './charts';
import ChartDataLabels from 'chartjs-plugin-datalabels';
import { chartColor, pageData } from './common';

// A bar per maturity bucket, labelled with its share of the total principal.
// Empty buckets get no label.
function drawShareBarChart(canvasId, labels, values, total, currency) {
    const share = value => (total ? `${(value / total * 100).toFixed(0)}%` : '');
    const options = barChartOptions(currency, {
        tooltipLabel: ctx => `${ctx.formattedValue} ${currency} (${share(ctx.raw)})`,
    });
    // Room for the share labels above the bars: text, label padding and offset.
    options.layout = { padding: { top: 28 } };
    options.plugins.datalabels = {
        display: ctx => ctx.dataset.data[ctx.dataIndex] !== 0,
        anchor: 'end',
        align: 'end',
        formatter: share,
        color: '#566171',
    };
    new Chart(document.getElementById(canvasId), {
        type: 'bar',
        data: { labels, datasets: [{ ...barStyle, data: values, backgroundColor: chartColor(0) }] },
        options,
        plugins: [ChartDataLabels],
    });
}

// The principal per bucket, and cumulated: repaid within each bucket's time to maturity.
function drawMaturityCharts(maturities) {
    const values = maturities.valueMicros.map(v => Math.round(v / 1e6));
    const total = values.reduce((sum, v) => sum + v, 0);
    let repaid = 0;
    const cumulative = values.map(v => (repaid += v));
    drawShareBarChart('maturities-canvas', maturities.bucketLabels, values, total, maturities.currency);
    drawShareBarChart('cumulative-maturities-canvas', maturities.bucketLabels, cumulative, total, maturities.currency);
}

export function init() {
    // There is no chart data when exchange rates are missing.
    const data = pageData('maturities-chart-data');
    if (data) drawMaturityCharts(data);
}
