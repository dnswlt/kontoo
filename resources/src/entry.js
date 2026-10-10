import { callout, calloutError, hideCallout, preventEnterSubmit } from './common';
import { initCombobox } from './combobox';

let assetInfoRequest;
let assetInfoFor = null; // { assetId, date } of the asset info shown or loading.
let typeCombobox;

function selectedAsset() {
    const id = document.getElementById('AssetID').value;
    return [...document.getElementById('AssetList').options].find(option => option.value === id);
}

function showAssetInfo(show, html = '') {
    const info = document.getElementById('asset-info');
    info.classList.toggle('hidden', !show);
    info.innerHTML = show ? html : '';
}

async function fetchAssetInfo({ force = false } = {}) {
    const asset = selectedAsset();
    const date = document.getElementById('ValueDate').value.trim();
    const parsedDate = /^\d{4}-\d{2}-\d{2}$/.test(date) ? new Date(`${date}T00:00:00Z`) : null;
    const validDate = parsedDate && !Number.isNaN(parsedDate.valueOf()) && parsedDate.toISOString().slice(0, 10) === date;
    if (!asset || !validDate || document.getElementById('Type').value === 'ExchangeRate') {
        assetInfoRequest?.abort();
        assetInfoFor = null;
        showAssetInfo(false);
        return;
    }
    if (!force && assetInfoFor?.assetId === asset.value && assetInfoFor.date === date) return;
    // Info for another asset is misleading; for another date it is fine until replaced.
    if (assetInfoFor?.assetId !== asset.value) showAssetInfo(false);
    assetInfoRequest?.abort();
    assetInfoFor = { assetId: asset.value, date };
    assetInfoRequest = new AbortController();
    try {
        const response = await fetch('/kontoo/entries/assetinfo', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ assetId: asset.value, date }),
            signal: assetInfoRequest.signal
        });
        if (!response.ok) throw new Error(`Could not load asset information (${response.status}).`);
        const data = await response.json();
        showAssetInfo(data.status === 'OK', data.innerHTML);
    } catch (error) {
        if (error.name !== 'AbortError') {
            assetInfoFor = null;
            showAssetInfo(false);
            console.error(error);
        }
    }
}

function updateAsset() {
    const asset = selectedAsset();
    const input = document.getElementById('AssetID');
    const name = document.getElementById('AssetName');
    name.textContent = asset ? asset.textContent.trim() : '';
    name.classList.toggle('hidden', !name.textContent);
    input.setCustomValidity(asset || !input.value ? '' : 'Choose an asset from the list.');
    const type = document.getElementById('Type');
    const validTypes = asset?.dataset.entryTypes.split(' ');
    const options = [...document.getElementById('TypeList').options];
    for (const option of options) {
        const valid = !validTypes || !option.value || validTypes.includes(option.value);
        option.disabled = !valid;
        option.hidden = !valid;
    }
    if (options.some(option => option.value === type.value && option.disabled)) type.value = '';
    typeCombobox?.refresh();
    const currency = document.querySelector('input[name="Currency"]');
    if (currency && asset) currency.value = asset.dataset.currency;
    updateFields();
    fetchAssetInfo();
}

function updateFields() {
    const typeInput = document.getElementById('Type');
    const value = typeInput.value;
    const validType = [...document.getElementById('TypeList').options].some(option => option.value === value && !option.disabled);
    typeInput.setCustomValidity(!value || validType ? '' : 'Choose a valid entry type.');
    const type = validType ? value : '';
    let fields;
    switch (type) {
        case 'ExchangeRate': fields = ['QuoteCurrency', 'Price']; break;
        case 'AccountBalance':
        case 'InterestPayment':
        case 'DividendPayment':
        case 'AssetMaturity': fields = ['AssetID', 'Value']; break;
        case 'AccountDebit':
        case 'AccountCredit':
            fields = ['AssetID', 'Value'];
            if (!document.querySelector('[name="SequenceNum"]') && selectedAsset()?.dataset.supportsRepeat === 'true') {
                fields.push('RepeatForMonths');
            }
            break;
        case 'AssetHolding': fields = ['AssetID', 'Value', 'Quantity', 'Price']; break;
        case 'AssetPrice': fields = ['AssetID', 'Price']; break;
        case 'AssetPurchase':
        case 'AssetSale': fields = ['AssetID', 'Value', 'Quantity', 'Price', 'Cost']; break;
        default: fields = ['AssetID'];
    }
    for (const name of ['AssetID', 'Value', 'Quantity', 'Price', 'Cost', 'QuoteCurrency', 'RepeatForMonths']) {
        const field = document.getElementById(`${name}Field`);
        const visible = fields.includes(name);
        field.classList.toggle('hidden', !visible);
        field.querySelectorAll('input, select').forEach(input => { input.disabled = !visible; });
    }
    document.getElementById('AssetID').required = type !== 'ExchangeRate';
    const currency = document.querySelector('input[name="Currency"]');
    if (currency && type === 'ExchangeRate') {
        currency.value = document.getElementById('ExchangeRateLabel')?.dataset.baseCurrency || currency.value;
    }
}

async function submitForm(event) {
    event.preventDefault();
    const form = event.currentTarget;
    if (form.getAttribute('aria-busy') === 'true') return;
    const addAnother = event.submitter?.value === 'next';
    const formData = new FormData(form);
    const entry = {};
    for (const [name, value] of formData) {
        if (value && name !== 'RepeatForMonths') entry[name] = name === 'SequenceNum' ? Number(value) : value;
    }
    const update = formData.has('SequenceNum');
    const buttons = form.querySelectorAll('button[type="submit"]');
    form.setAttribute('aria-busy', 'true');
    buttons.forEach(button => { button.disabled = true; });
    hideCallout();
    try {
        const response = await fetch(form.action, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ updateExisting: update, repeatForMonths: Number(formData.get('RepeatForMonths') || 0), entry })
        });
        if (!response.ok) throw new Error((await response.text()) || `Save failed (${response.status}).`);
        const data = await response.json();
        if (data.status !== 'OK') throw new Error(data.error || 'Could not save the entry.');
        if (!addAnother || update) {
            window.location.assign(form.dataset.returnUrl);
            return;
        }
        callout(`Added entry #${data.sequenceNum}. Ready for another entry.`);
        // Keep the asset, date and type for batch entry; clear transaction-specific values.
        for (const name of ['Value', 'Quantity', 'Price', 'Cost', 'Comment']) document.getElementById(name).value = '';
        document.getElementById('RepeatForMonths').value = '0';
        fetchAssetInfo({ force: true });
        form.querySelector('#ValueField:not(.hidden) input, #QuantityField:not(.hidden) input, #PriceField:not(.hidden) input')?.focus();
    } catch (error) {
        calloutError(error.message);
    } finally {
        form.removeAttribute('aria-busy');
        buttons.forEach(button => { button.disabled = false; });
    }
}

export function init() {
    const form = document.getElementById('entry-form');
    form.addEventListener('submit', submitForm);
    preventEnterSubmit(form);
    initCombobox(document.getElementById('AssetID'), { showValue: true, autocomplete: true });
    typeCombobox = initCombobox(document.getElementById('Type'), { autocomplete: true });
    document.getElementById('AssetID').addEventListener('input', updateAsset);
    document.getElementById('Type').addEventListener('input', () => {
        updateFields();
        fetchAssetInfo();
    });
    const quoteCurrency = document.getElementById('QuoteCurrency');
    if (quoteCurrency) {
        const updateLabel = () => {
            const label = document.getElementById('ExchangeRateLabel');
            label.textContent = `${label.dataset.baseCurrency}/${quoteCurrency.value}`;
        };
        quoteCurrency.addEventListener('input', updateLabel);
        updateLabel();
    }
    updateAsset();
    // fetchAssetInfo() skips the request unless asset or date changed.
    const date = document.getElementById('ValueDate');
    const dateChanged = () => fetchAssetInfo();
    date.addEventListener('input', dateChanged);
    date.addEventListener('change', dateChanged);
    date.addEventListener('blur', dateChanged);
    date._flatpickr.config.onValueUpdate.push(dateChanged);
}
