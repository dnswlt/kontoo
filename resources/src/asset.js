import { callout, calloutError, hideCallout, preventEnterSubmit } from './common';
import { initCombobox } from './combobox';

function typeOptions() {
    return [...document.getElementById('TypeList').options];
}

// Fields that only apply to some asset types, as listed by each type's data-fields.
function typeFields() {
    return new Set(typeOptions().flatMap(option => option.dataset.fields.split(' ')).filter(Boolean));
}

function selectedType() {
    const value = document.getElementById('Type').value;
    return typeOptions().find(option => option.value === value && !option.disabled);
}

function initiallySet(input) {
    if (input instanceof HTMLSelectElement) return [...input.options].some(option => option.defaultSelected && option.value);
    return input.defaultValue !== '';
}

function updateFields() {
    const typeInput = document.getElementById('Type');
    const type = selectedType();
    typeInput.setCustomValidity(type || !typeInput.value ? '' : 'Choose an asset type from the list.');
    const fields = type ? type.dataset.fields.split(' ') : [];
    for (const name of typeFields()) {
        const input = document.getElementById(name);
        // Fields that had a value when the page loaded stay, so editing never drops data unseen.
        const visible = fields.includes(name) || initiallySet(input);
        document.getElementById(`${name}Field`).classList.toggle('hidden', !visible);
        input.disabled = !visible;
    }
    const terms = document.getElementById('TermsSection');
    terms.hidden = !terms.querySelector('.field:not(.hidden)');
}

function clearForNextAsset(form) {
    // Keep type and currency for batch entry; clear everything else.
    for (const input of form.querySelectorAll('input[type="text"], textarea, select')) {
        if (input.id === 'Type' || input.id === 'Currency') continue;
        if (input._flatpickr) input._flatpickr.clear();
        else input.value = '';
    }
    document.getElementById('Name').focus();
}

async function submitForm(event) {
    event.preventDefault();
    const form = event.currentTarget;
    if (form.getAttribute('aria-busy') === 'true') return;
    const addAnother = event.submitter?.value === 'next';
    const formData = new FormData(form);
    const assetId = formData.get('AssetId') || '';
    formData.delete('AssetId'); // Not a field of the asset.
    const asset = {};
    for (const [name, raw] of formData) {
        const value = raw.trim();
        if (value) asset[name] = name === 'QuoteServiceSymbols' ? { YF: value } : value;
    }
    const buttons = form.querySelectorAll('button[type="submit"]');
    form.setAttribute('aria-busy', 'true');
    buttons.forEach(button => { button.disabled = true; });
    hideCallout();
    try {
        const response = await fetch(form.action, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ assetId, asset })
        });
        if (!response.ok) throw new Error((await response.text()) || `Save failed (${response.status}).`);
        const data = await response.json();
        if (data.status !== 'OK') throw new Error(data.error || 'Could not save the asset.');
        if (!addAnother || assetId) {
            window.location.assign(form.dataset.returnUrl);
            return;
        }
        callout(`Added asset ${data.assetId}. Ready for another asset.`);
        clearForNextAsset(form);
    } catch (error) {
        calloutError(error.message);
    } finally {
        form.removeAttribute('aria-busy');
        buttons.forEach(button => { button.disabled = false; });
    }
}

export function init() {
    const form = document.getElementById('asset-form');
    form.addEventListener('submit', submitForm);
    preventEnterSubmit(form);
    const type = document.getElementById('Type');
    initCombobox(type, { autocomplete: true, showValue: true });
    type.addEventListener('input', updateFields);
    const currency = document.getElementById('Currency');
    initCombobox(currency);
    currency.addEventListener('input', () => {
        const upper = currency.value.toUpperCase();
        if (upper === currency.value) return;
        const { selectionStart, selectionEnd } = currency;
        currency.value = upper;
        currency.setSelectionRange(selectionStart, selectionEnd);
    });
    const interest = document.getElementById('Interest');
    // A bare number means percent: "2.5" is 2.5%, not 250%.
    interest.addEventListener('change', () => {
        if (/^\d+(\.\d+)?$/.test(interest.value)) interest.value += '%';
    });
    updateFields();
}
