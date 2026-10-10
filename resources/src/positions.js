import { base64ToString, chartColor, loadPref, savePref, stringToBase64 } from './common';
import Chart, { barChartOptions, barStyle, lossColor } from './charts';
import ChartDataLabels from 'chartjs-plugin-datalabels';
import 'chartjs-adapter-date-fns';
import { enGB } from 'date-fns/locale';

// The chart shows the total value of all assets, or only the selected assets.
// Selection and period live in the URL. The last chosen period (the default for
// URLs without one) is a per-browser preference.
const periodKey = 'kontoo.positions.chartPeriod';
const compactNumber = new Intl.NumberFormat('en', { notation: 'compact' });

let state;
let chart = null;
let currency = '';
let request = null;

function readState() {
    state = { assetIds: [], period: loadPref(periodKey) || '1Y' };
    const param = new URLSearchParams(window.location.search).get('ui-state');
    if (!param) return;
    try {
        state = { ...state, ...JSON.parse(base64ToString(param)) };
    } catch (error) {
        console.error('Invalid ui-state= param:', error);
    }
}

function updateState(update) {
    state = { ...state, ...update };
    const url = new URL(window.location);
    url.searchParams.set('ui-state', stringToBase64(JSON.stringify(state)));
    window.history.pushState({}, '', url);
    render();
}

function setPeriod(period) {
    savePref(periodKey, period);
    updateState({ period });
}

function toggleAsset(assetId) {
    const assetIds = state.assetIds.includes(assetId)
        ? state.assetIds.filter(id => id !== assetId)
        : [...state.assetIds, assetId];
    updateState({ assetIds });
}

function formatMoney(value) {
    return value.toFixed(2).replace(/\B(?=(\d{3})+(?!\d))/g, "'");
}

function dot(seriesColor) {
    const span = document.createElement('span');
    span.className = 'chart-dot';
    span.style.setProperty('--series-color', seriesColor);
    return span;
}

function drawChart(timelines) {
    const datasets = timelines.map((timeline, i) => ({
        label: timeline.label,
        borderColor: chartColor(i),
        backgroundColor: chartColor(i),
        data: timeline.timestamps.map((x, j) => ({ x, y: timeline.valueMicros[j] / 1e6 })),
    }));
    if (!chart) {
        chart = new Chart(document.getElementById('positions-canvas'), {
            type: 'line',
            data: { datasets },
            options: {
                animation: false,
                parsing: false,
                maintainAspectRatio: false,
                interaction: { mode: 'nearest', axis: 'x', intersect: false },
                elements: {
                    line: { stepped: true, borderWidth: 2 },
                    point: { radius: 0, hoverRadius: 4 },
                },
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        callbacks: {
                            label: ctx => `${ctx.dataset.label}: ${formatMoney(ctx.parsed.y)} ${currency}`,
                        },
                    },
                },
                scales: {
                    x: {
                        type: 'time',
                        adapters: { date: { locale: enGB } },
                        time: {
                            displayFormats: { day: 'd MMM', month: 'MMM yy' },
                            tooltipFormat: 'd MMM yyyy',
                        },
                    },
                    y: {
                        beginAtZero: true,
                        ticks: { callback: value => compactNumber.format(value) },
                    },
                },
            },
        });
    }
    chart.data.datasets = datasets;
    chart.update('none');
}

// The series header shows the total's title, or one removable chip per selected asset.
function renderSeries(timelines) {
    const series = document.getElementById('positions-chart-series');
    series.replaceChildren();
    if (state.assetIds.length === 0) {
        const title = document.createElement('span');
        title.className = 'positions-chart-title';
        title.append(dot(chartColor(0)), `Total value (${currency})`);
        series.append(title);
        return;
    }
    timelines.forEach((timeline, i) => {
        const chip = document.createElement('button');
        chip.type = 'button';
        chip.className = 'chart-chip';
        chip.setAttribute('aria-label', `Remove ${timeline.label} from chart`);
        const remove = document.createElement('span');
        remove.setAttribute('aria-hidden', 'true');
        remove.textContent = '×';
        chip.append(dot(chartColor(i)), timeline.label, remove);
        chip.addEventListener('click', () => toggleAsset(timeline.assetId));
        series.append(chip);
    });
    const total = document.createElement('button');
    total.type = 'button';
    total.className = 'chart-show-total';
    total.textContent = 'Show total';
    total.addEventListener('click', () => updateState({ assetIds: [] }));
    series.append(total);
}

// Selected rows get a dot in their line's color, so the table doubles as legend.
function markRows(timelines) {
    document.querySelectorAll('#positions-table .chart-dot').forEach(d => d.remove());
    timelines.forEach((timeline, i) => {
        if (!timeline.assetId) return; // The total has no row.
        const cell = document.querySelector(`[data-position-chart="${CSS.escape(timeline.assetId)}"]`)?.closest('td');
        cell?.prepend(dot(chartColor(i)));
    });
}

async function render() {
    document.querySelectorAll('.positions-chart-periods button').forEach(button => {
        button.setAttribute('aria-pressed', String(button.dataset.period === state.period));
    });
    document.querySelectorAll('[data-position-chart]').forEach(button => {
        button.textContent = state.assetIds.includes(button.dataset.positionChart) ? 'Remove from chart' : 'Show in chart';
    });
    request?.abort();
    request = new AbortController();
    try {
        const date = new URLSearchParams(window.location.search).get('date');
        const response = await fetch('/kontoo/positions/timeline', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                assetIds: state.assetIds,
                endTimestamp: date ? new Date(date).getTime() : Date.now(),
                period: state.period,
            }),
            signal: request.signal,
        });
        const result = await response.json();
        if (result.status !== 'OK') throw new Error(result.error);
        currency = result.currency;
        drawChart(result.timelines);
        renderSeries(result.timelines);
        markRows(result.timelines);
    } catch (error) {
        if (error.name !== 'AbortError') console.error('Error fetching timeline:', error);
    }
}

// Category allocation: one horizontal bar per category, labelled with its share
// of the net total. Debts are negative and extend left of zero.
function drawCategoryChart(data) {
    const values = data.values.map(v => Math.round(v / 1e6));
    const total = values.reduce((sum, v) => sum + v, 0);
    const share = value => (total ? `${(value / total * 100).toFixed(1)}%` : '');
    const canvas = document.getElementById('category-canvas');
    canvas.parentElement.style.height = `${values.length * 34 + 40}px`;
    const options = barChartOptions(data.currency, {
        horizontal: true,
        tooltipLabel: ctx => `${ctx.formattedValue} ${data.currency} (${share(ctx.raw)})`,
    });
    options.scales.x.grace = '15%'; // Room for the share labels at the bar ends.
    options.plugins.datalabels = { anchor: 'end', align: 'end', formatter: share, color: '#566171' };
    new Chart(canvas, {
        type: 'bar',
        data: {
            labels: data.labels,
            datasets: [{ ...barStyle, data: values, backgroundColor: values.map(v => (v < 0 ? lossColor : chartColor(0))) }],
        },
        options,
        plugins: [ChartDataLabels],
    });
}

export function init() {
    const categoryData = document.getElementById('category-chart-data');
    if (categoryData) drawCategoryChart(JSON.parse(categoryData.textContent));
    document.querySelectorAll('[data-position-chart]').forEach(button => {
        button.addEventListener('click', () => {
            button.closest('[popover]').hidePopover();
            toggleAsset(button.dataset.positionChart);
        });
    });
    document.querySelectorAll('.positions-chart-periods button').forEach(button => {
        button.addEventListener('click', () => setPeriod(button.dataset.period));
    });
    window.addEventListener('popstate', () => {
        readState();
        render();
    });
    readState();
    render();
}
