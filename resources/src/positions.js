import { chartColor, loadPref, pageData, savePref } from './common';
import Chart, { barChartOptions, barStyle, lossColor } from './charts';
import { color } from 'chart.js/helpers';
import ChartDataLabels from 'chartjs-plugin-datalabels';
import 'chartjs-adapter-date-fns';
import { enGB } from 'date-fns/locale';

// The value chart shows the total value of all assets, or only the selected
// assets. Its state lives in the URL (period=, repeated chart=), so Back/Forward
// and bookmarks work. The page embeds the chart's initial data; changes fetch the
// data for the updated URL without reloading the page. The last chosen period is
// a cookie, so the server can use it for URLs without a period.
const periodCookie = 'kontoo_chart_period';
const compactNumber = new Intl.NumberFormat('en', { notation: 'compact' });
const wholeNumber = new Intl.NumberFormat('en', { maximumFractionDigits: 0 });

let state = { period: '1Y', assetIds: [] };
let chart = null;
let currency = '';
let request = null;

function setChartParams(url, { period, assetIds }) {
    url.searchParams.delete('ui-state'); // Replaced by period= and chart=.
    url.searchParams.set('period', period);
    url.searchParams.delete('chart');
    assetIds.forEach(id => url.searchParams.append('chart', id));
}

// Records the new state in the URL, as a new history entry, and loads its chart.
function updateState(update) {
    const url = new URL(window.location);
    setChartParams(url, { ...state, ...update });
    window.history.pushState({}, '', url);
    load();
}

function setPeriod(period) {
    document.cookie = `${periodCookie}=${period}; path=/kontoo; max-age=${365 * 24 * 3600}; samesite=lax`;
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
        title.append(dot(chartColor(0)), 'Total');
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

// Fetches the chart data for the current URL: the endpoint takes the page's parameters.
async function load() {
    request?.abort();
    request = new AbortController();
    try {
        const response = await fetch(`/kontoo/positions/timeline${window.location.search}`, { signal: request.signal });
        if (!response.ok) throw new Error((await response.text()) || `HTTP error: ${response.status}`);
        show(await response.json());
    } catch (error) {
        if (error.name !== 'AbortError') console.error('Error fetching timeline:', error);
    }
}

// Shows the chart data and the state the server computed it for.
function show(data) {
    state = { period: data.period, assetIds: data.assetIds };
    currency = data.currency;
    document.querySelectorAll('.positions-chart-periods button').forEach(button => {
        button.setAttribute('aria-pressed', String(button.dataset.period === state.period));
    });
    document.querySelectorAll('[data-position-chart]').forEach(button => {
        button.textContent = state.assetIds.includes(button.dataset.positionChart) ? 'Remove from chart' : 'Show in chart';
    });
    drawChart(data.timelines);
    renderSeries(data.timelines);
    markRows(data.timelines);
    // The date controls were rendered for the URL at page load: keep their chart state current.
    document.querySelectorAll('.positions-controls a.report-date-option, .positions-controls a.report-today').forEach(link => {
        const url = new URL(link.href);
        setChartParams(url, state);
        link.href = url;
    });
}

// Category allocation: one horizontal bar per category, labelled with its share
// of the total of the categories shown. Debts are negative and extend left of zero.
// Categories can be excluded with toggle buttons; this is remembered per browser.
const excludedCategoriesKey = 'kontoo.positions.excludedCategories';

function loadExcludedCategories() {
    try {
        return new Set(JSON.parse(loadPref(excludedCategoriesKey) || '[]'));
    } catch {
        return new Set();
    }
}

// With several currencies, each bar is stacked by currency (values in the base
// currency) and the share label sits at the end of the whole bar.
function initCategoryChart(data) {
    const excluded = loadExcludedCategories();
    const canvas = document.getElementById('category-canvas');
    let totals = []; // Per category shown.
    let total = 0;
    let outermost = []; // Per category shown: the dataset whose segment ends the bar.
    const share = value => (total ? `${(value / total * 100).toFixed(1)}%` : '');
    const options = barChartOptions(data.currency, { horizontal: true });
    options.scales.x.stacked = true;
    options.scales.y.stacked = true;
    options.scales.x.grace = '15%'; // Room for the share labels at the bar ends.
    options.interaction = { mode: 'index', axis: 'y', intersect: false };
    options.plugins.legend = {
        position: 'bottom',
        onClick: () => {}, // A key for the colors only; categories have their own toggles.
        labels: { usePointStyle: true, pointStyle: 'circle', boxWidth: 8, boxHeight: 8 },
    };
    options.plugins.tooltip = {
        filter: item => item.raw !== 0,
        callbacks: {
            label: ctx => `${ctx.dataset.label}: ${ctx.formattedValue} ${data.currency}`,
            footer: items => {
                const value = totals[items[0]?.dataIndex];
                return value !== undefined ? `Total: ${wholeNumber.format(value)} ${data.currency} (${share(value)})` : '';
            },
        },
    };
    options.plugins.datalabels = {
        display: ctx => outermost[ctx.dataIndex] === ctx.datasetIndex,
        formatter: (value, ctx) => share(totals[ctx.dataIndex]),
        anchor: 'end',
        align: 'end',
        color: '#566171',
    };
    const chart = new Chart(canvas, { type: 'bar', data: {}, options, plugins: [ChartDataLabels] });

    const render = () => {
        const shown = data.labels.map((_, i) => i).filter(i => !excluded.has(data.labels[i]));
        totals = shown.map(i => Math.round(data.values[i] / 1e6));
        total = totals.reduce((sum, v) => sum + v, 0);
        const datasets = data.currencies.map((currency, k) => {
            const values = shown.map(i => Math.round(data.valuesByCurrency[k][i] / 1e6));
            return {
                ...barStyle,
                label: currency,
                data: values,
                backgroundColor: chartColor(k),
                borderRadius: ctx => (outermost[ctx.dataIndex] === ctx.datasetIndex ? barStyle.borderRadius : 0),
            };
        }).filter(d => d.data.some(v => v !== 0)); // Only currencies held in the categories shown.
        outermost = shown.map((_, j) => datasets.reduce((last, d, k) => (d.data[j] !== 0 ? k : last), 0));
        // The legend only helps when bars are stacked by more than one currency.
        const showLegend = datasets.length > 1;
        chart.options.plugins.legend.display = showLegend;
        canvas.parentElement.style.height = `${shown.length * 34 + (showLegend ? 70 : 40)}px`;
        chart.data = { labels: shown.map(i => data.labels[i]), datasets };
        chart.update('none');
    };

    const toggles = data.labels.map(label => {
        const button = document.createElement('button');
        button.type = 'button';
        button.textContent = label;
        button.setAttribute('aria-pressed', String(!excluded.has(label)));
        button.addEventListener('click', () => {
            if (excluded.has(label)) excluded.delete(label);
            else excluded.add(label);
            button.setAttribute('aria-pressed', String(!excluded.has(label)));
            savePref(excludedCategoriesKey, JSON.stringify([...excluded]));
            render();
        });
        return button;
    });
    document.getElementById('category-toggles').replaceChildren(...toggles);
    render();
}

// Yearly change: one bar per year with the change in total value since the end
// of the previous year, labelled with the amount; the tooltip adds the change in
// percent. The current year runs to the report date and has a lighter bar.
function drawYearlyChangeChart(data) {
    const changes = data.changeMicros.map(v => Math.round(v / 1e6));
    const values = data.valueMicros.map(v => Math.round(v / 1e6));
    // In percent of the previous year-end value, if that was positive.
    const percent = i => {
        const previous = values[i] - changes[i];
        return previous > 0 ? changes[i] / previous * 100 : null;
    };
    const barColor = ctx => {
        const c = changes[ctx.dataIndex] < 0 ? lossColor : chartColor(0); // The color of the total in the value chart.
        return data.labels[ctx.dataIndex].endsWith('YTD') ? color(c).alpha(0.5).rgbString() : c;
    };
    const options = barChartOptions(data.currency);
    options.plugins.tooltip.callbacks.label = ctx => {
        const p = percent(ctx.dataIndex);
        return `${wholeNumber.format(changes[ctx.dataIndex])} ${data.currency}${p === null ? '' : ` (${p > 0 ? '+' : ''}${p.toFixed(1)}%)`}`;
    };
    options.plugins.tooltip.callbacks.footer = items => `Value: ${wholeNumber.format(values[items[0].dataIndex])} ${data.currency}`;
    // Room for the amount labels at the bar ends, above and below (Chart.js takes
    // the percentage of half the data range).
    options.scales.y.grace = '40%';
    options.plugins.datalabels = {
        formatter: value => `${value > 0 ? '+' : ''}${compactNumber.format(value)}`,
        // The plugin's direction for vertical bars is always up: the end of a
        // negative bar is its start.
        anchor: ctx => (changes[ctx.dataIndex] < 0 ? 'start' : 'end'),
        align: ctx => (changes[ctx.dataIndex] < 0 ? 'start' : 'end'),
        color: '#566171',
    };
    new Chart(document.getElementById('yearly-change-canvas'), {
        type: 'bar',
        data: { labels: data.labels, datasets: [{ ...barStyle, data: changes, backgroundColor: barColor }] },
        options,
        plugins: [ChartDataLabels],
    });
}

export function init() {
    const categoryData = pageData('category-chart-data');
    if (categoryData) initCategoryChart(categoryData);
    const yearlyChangeData = pageData('yearly-change-chart-data');
    if (yearlyChangeData) drawYearlyChangeChart(yearlyChangeData);
    document.querySelectorAll('[data-position-chart]').forEach(button => {
        button.addEventListener('click', () => {
            button.closest('[popover]').hidePopover();
            toggleAsset(button.dataset.positionChart);
        });
    });
    document.querySelectorAll('.positions-chart-periods button').forEach(button => {
        button.addEventListener('click', () => setPeriod(button.dataset.period));
    });
    window.addEventListener('popstate', load);
    show(pageData('value-chart-data'));
}
