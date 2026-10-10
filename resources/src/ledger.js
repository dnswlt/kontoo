import flatpickr from 'flatpickr';
import { calloutError } from './common';

async function deleteLedgerEntry(button) {
    const sequenceNum = Number(button.dataset.seq);
    if (!window.confirm(`Delete entry #${sequenceNum}? This cannot be undone.`)) return;
    button.disabled = true;
    try {
        const response = await fetch('/kontoo/entries/delete', {
            method: 'POST',
            body: JSON.stringify({ sequenceNum }),
            headers: { 'Content-Type': 'application/json' }
        });
        if (!response.ok) throw new Error((await response.text()) || `HTTP error: ${response.status}`);
        const data = await response.json();
        if (data.status !== 'OK') throw new Error(data.error || 'Could not delete the entry.');
        location.reload();
    } catch (error) {
        calloutError(error.message);
        button.disabled = false;
    }
}

async function reloadLedger(button) {
    button.disabled = true;
    try {
        const response = await fetch('/kontoo/ledger/reload', { method: 'POST' });
        if (!response.ok) throw new Error((await response.text()) || `HTTP error: ${response.status}`);
        location.reload();
    } catch (error) {
        calloutError(error.message);
        button.disabled = false;
    }
}

export function init() {
    document.querySelectorAll('#ledger-page form').forEach(form => {
        form.addEventListener('formdata', event => {
            for (const [name, value] of [...event.formData]) {
                if (value === '') event.formData.delete(name);
            }
        });
    });
    document.querySelectorAll('button.delete').forEach(button => {
        button.addEventListener('click', () => deleteLedgerEntry(button));
    });
    const reload = document.getElementById('reload-ledger');
    reload.addEventListener('click', () => reloadLedger(reload));
    const form = document.getElementById('ledger-filters');
    if (form) {
        const includeMarket = document.getElementById('include-market');
        includeMarket.addEventListener('click', () => {
            form.elements.include_market.value = includeMarket.getAttribute('aria-pressed') === 'true' ? '0' : '1';
            form.requestSubmit();
        });
        const search = form.querySelector('.ledger-asset-search');
        search.addEventListener('input', () => {
            const term = search.value.trim().toLowerCase();
            let matches = 0;
            form.querySelectorAll('.ledger-asset-options label').forEach(option => {
                const visible = option.textContent.toLowerCase().includes(term);
                option.hidden = !visible;
                if (visible) matches++;
            });
            form.querySelector('.ledger-asset-empty').hidden = matches > 0;
        });
        search.addEventListener('keydown', event => {
            if (event.key === 'Enter') event.preventDefault();
        });
        // Render the calendars inside the date popover: clicks on a calendar
        // appended to the body would light-dismiss the popover.
        form.querySelectorAll('.ledger-date-input').forEach(input => flatpickr(input, { allowInput: true, static: true }));
    }
    document.addEventListener('keydown', event => {
        const target = event.target;
        if (event.key === '/' && !event.ctrlKey && !event.metaKey && !event.altKey &&
            !target.isContentEditable && !['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON'].includes(target.tagName)) {
            event.preventDefault();
            document.getElementById('filter').focus();
        }
    });
}
