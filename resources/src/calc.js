import { calloutError, calloutWarning, hideCallout, loadPref, parseMicros, savePref } from './common';

function initIrr(form) {
    const result = document.getElementById('IRR');
    form.addEventListener('submit', async event => {
        event.preventDefault();
        const value = id => document.getElementById(id).value.trim();
        try {
            const response = await fetch('/kontoo/calculate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    nominalValue: value('NominalValue'),
                    purchasePrice: value('PurchasePrice'),
                    cost: value('Cost'),
                    purchaseDate: value('PurchaseDate'),
                    maturityDate: value('MaturityDate'),
                    interestRate: value('InterestRate'),
                    accruedInterest: false,
                }),
            });
            if (!response.ok) throw new Error((await response.text()) || `Calculation failed (${response.status}).`);
            const data = await response.json();
            if (data.status !== 'OK') throw new Error(data.error);
            hideCallout();
            result.textContent = data.irrFormatted;
        } catch (error) {
            result.textContent = '–';
            calloutError(error.message);
        }
    });
}

// Allocation planner: a target share for each category in the plan, and how
// much to buy or sell of each to reach them, also when investing (or
// withdrawing) an amount. The server calculates the plan; this page shows the
// current shares and keeps the targets at 100% (in integer micros, so they add
// up exactly). The categories in the plan, the targets and the mode are
// remembered per browser.
const allocationKey = 'kontoo.calc.allocation';

const sum = values => values.reduce((total, v) => total + v, 0);

// Formats a share in micros as a percentage, e.g. 145000 as "14.5%".
const formatPercent = micros => `${micros / 1e4}%`;

// Formats micros like the server's money format: cut off at the cent.
function formatMoney(micros) {
    const cents = Math.trunc(micros / 1e4);
    const text = (Math.abs(cents) / 100).toFixed(2).replace(/\B(?=(\d{3})+(?!\d))/g, "'");
    return cents < 0 ? `(${text})` : text;
}

function initAllocation(form) {
    const categories = [...form.querySelectorAll('tbody tr')].map(row => ({
        name: row.dataset.category,
        value: Number(row.dataset.value),
        row,
        input: row.querySelector('input'),
        fill: row.querySelector('.fill-rest'),
    }));
    const amountInput = document.getElementById('AllocationAmount');
    const toggles = [...document.querySelectorAll('#allocation-toggles button')];
    const modeButtons = [...document.querySelectorAll('#allocation-mode button')];
    const foot = form.querySelector('tfoot tr');

    let saved = {};
    try {
        saved = JSON.parse(loadPref(allocationKey)) ?? {};
    } catch {
        // Start with the defaults.
    }
    const excluded = new Set(saved.excluded ?? categories.filter(c => !('included' in c.row.dataset)).map(c => c.name));
    let mode = saved.mode === 'buy' ? 'buy' : 'rebalance';
    categories.forEach(c => { c.input.value = saved.targets?.[c.name] ?? ''; });
    const save = () => savePref(allocationKey, JSON.stringify({
        excluded: [...excluded],
        targets: Object.fromEntries(categories.filter(c => c.input.value.trim()).map(c => [c.name, c.input.value])),
        mode,
    }));

    const included = () => categories.filter(c => !excluded.has(c.name));
    const target = c => (c.input.value.trim() === '' ? 0 : parseMicros(c.input.value)); // NaN if invalid.
    const setAmount = (cell, text) => {
        cell.textContent = text || '–';
        cell.classList.toggle('negative-amount', Boolean(text?.startsWith('('))); // The money format's brackets.
    };
    const showResult = (row, result) => {
        setAmount(row.querySelector('[data-change]'), result?.change);
        setAmount(row.querySelector('[data-new-value]'), result?.newValue);
        row.querySelector('[data-new-share]').textContent = result?.newShare || '–';
    };

    // Shows the categories in the plan with their current shares and the sum of
    // the targets. Clears the results, which no longer match the inputs.
    let version = 0;
    const update = () => {
        version++;
        hideCallout();
        categories.forEach(c => { c.row.hidden = excluded.has(c.name); });
        toggles.forEach(b => b.setAttribute('aria-pressed', String(!excluded.has(b.dataset.category))));
        modeButtons.forEach(b => b.setAttribute('aria-pressed', String(b.dataset.mode === mode)));
        const shown = included();
        const total = sum(shown.map(c => c.value));
        shown.forEach(c => {
            c.row.querySelector('[data-share]').textContent = total ? `${(c.value / total * 100).toFixed(1)}%` : '–';
            showResult(c.row, null);
        });
        setAmount(foot.querySelector('[data-value]'), formatMoney(total));
        foot.querySelector('[data-share]').textContent = total ? '100.0%' : '–';
        const targetSum = sum(shown.map(target));
        const targetCell = foot.querySelector('[data-target]');
        targetCell.textContent = Number.isNaN(targetSum) ? '–' : formatPercent(targetSum);
        targetCell.classList.toggle('negative-amount', targetSum !== 1e6);
        showResult(foot, null);
    };

    toggles.forEach(button => button.addEventListener('click', () => {
        const name = button.dataset.category;
        if (excluded.has(name)) excluded.delete(name);
        else excluded.add(name);
        save();
        update();
    }));
    modeButtons.forEach(button => button.addEventListener('click', () => {
        mode = button.dataset.mode;
        save();
        update();
    }));
    categories.forEach(c => {
        c.input.addEventListener('input', () => {
            save();
            update();
        });
        // Sets the target to what the others leave of 100%.
        c.fill.addEventListener('click', () => {
            const others = sum(included().filter(o => o !== c).map(target));
            if (Number.isNaN(others)) return; // The invalid field is marked.
            c.input.value = formatPercent(1e6 - others);
            c.input.classList.remove('invalid');
            c.input.setCustomValidity('');
            save();
            update();
        });
    });
    amountInput.addEventListener('input', update);

    form.addEventListener('submit', async event => {
        event.preventDefault();
        const shown = included();
        const targetSum = sum(shown.map(target));
        if (targetSum !== 1e6) {
            calloutWarning(`The targets add up to ${formatPercent(targetSum)}, not 100%.`);
            return;
        }
        const requested = version;
        try {
            const response = await fetch('/kontoo/calculate/allocation', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    targets: shown.map(c => ({ category: c.name, target: c.input.value.trim() || '0' })),
                    amount: amountInput.value.trim() || '0',
                    buyOnly: mode === 'buy',
                }),
            });
            if (!response.ok) throw new Error((await response.text()) || `Calculation failed (${response.status}).`);
            const data = await response.json();
            if (requested !== version) return; // The inputs changed meanwhile.
            if (data.status !== 'OK') {
                calloutWarning(data.error);
                return;
            }
            shown.forEach((c, i) => showResult(c.row, data.rows[i]));
            showResult(foot, data.total);
        } catch (error) {
            calloutError(error.message);
        }
    });

    update();
}

export function init() {
    const irrForm = document.getElementById('irr-form');
    if (irrForm) initIrr(irrForm);
    const allocationForm = document.getElementById('allocation-form');
    if (allocationForm) initAllocation(allocationForm);
}
