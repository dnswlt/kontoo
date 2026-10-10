// Chart.js configured to match the page: same font, muted text, light grid lines.
// All chart modules import Chart from here.
import Chart from 'chart.js/auto';

Chart.defaults.font.family = getComputedStyle(document.body).fontFamily;
Chart.defaults.font.size = 12;
Chart.defaults.color = '#566171';
Chart.defaults.borderColor = '#eef1f5';

const compactNumber = new Intl.NumberFormat('en', { notation: 'compact' });

// Options for bar charts of amounts in currency: light horizontal grid with
// a stronger zero line, compact axis labels, the currency in the tooltip.
export function barChartOptions(currency, tooltipLabel = ctx => `${ctx.formattedValue} ${currency}`) {
    return {
        animation: false,
        maintainAspectRatio: false,
        plugins: {
            legend: { display: false },
            tooltip: { callbacks: { label: tooltipLabel } },
        },
        scales: {
            x: { grid: { display: false }, border: { display: false } },
            y: {
                border: { display: false },
                grid: { color: ctx => (ctx.tick.value === 0 ? '#aab7c7' : '#eef1f5') },
                ticks: { callback: value => compactNumber.format(value) },
            },
        },
    };
}

// Dataset styling for bar charts.
export const barStyle = { borderRadius: 4, maxBarThickness: 36 };

export default Chart;
