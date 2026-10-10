// Chart.js configured to match the page: same font, muted text, light grid lines.
// All chart modules import Chart from here.
import Chart from 'chart.js/auto';

Chart.defaults.font.family = getComputedStyle(document.body).fontFamily;
Chart.defaults.font.size = 12;
Chart.defaults.color = '#566171';
Chart.defaults.borderColor = '#eef1f5';

const compactNumber = new Intl.NumberFormat('en', { notation: 'compact' });

// Colors for positive and negative amounts (--red-number for losses).
export const gainColor = '#2f9e6e';
export const lossColor = 'rgb(219, 18, 34)';

// Options for bar charts of amounts in currency: light grid along the value axis
// with a stronger zero line, compact axis labels, the currency in the tooltip.
// Horizontal charts have the categories on the y axis.
export function barChartOptions(currency, { tooltipLabel = ctx => `${ctx.formattedValue} ${currency}`, horizontal = false } = {}) {
    const valueAxis = {
        border: { display: false },
        grid: { color: ctx => (ctx.tick.value === 0 ? '#aab7c7' : '#eef1f5') },
        ticks: { callback: value => compactNumber.format(value) },
    };
    const categoryAxis = { grid: { display: false }, border: { display: false } };
    return {
        indexAxis: horizontal ? 'y' : 'x',
        animation: false,
        maintainAspectRatio: false,
        plugins: {
            legend: { display: false },
            tooltip: { callbacks: { label: tooltipLabel } },
        },
        scales: horizontal ? { x: valueAxis, y: categoryAxis } : { x: categoryAxis, y: valueAxis },
    };
}

// Dataset styling for bar charts.
export const barStyle = { borderRadius: 4, maxBarThickness: 36 };

export default Chart;
