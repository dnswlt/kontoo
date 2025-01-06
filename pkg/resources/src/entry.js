import { callout, calloutStatus } from "./common";


function inputAutocomplete(event, callback) {
    const input = event.target;
    if (event.inputType && event.inputType.startsWith("deleteContent")) {
        // Don't auto-complete if Backspace or Del were hit.
        return;
    }
    const token = input.value.toLowerCase();
    if (token.length < 3) {
        return;  // Require at least 3 characters before trying to match.
    }
    const datalistId = input.getAttribute('list');
    const options = document.querySelectorAll(`#${datalistId} option`);
    let id = null;
    for (const opt of options) {
        const searchText = (opt.value + " " + opt.textContent).toLowerCase();
        if (opt.value === input.value) {
            id = opt.value;
            break;
        }
        if (searchText.includes(token)) {
            if (id) {
                // no unique match
                return;
            }
            id = opt.value;
        }
    }
    if (!id) {
        return; // No match
    }
    input.value = id;
    callback(id);
}

async function fetchAssetInfo(assetId) {
    if (!assetId) {
        assetId = document.querySelector("#AssetID").value;
        if (!assetId) {
            return;
        }
    }
    const date = document.querySelector("#ValueDate").value;
    try {
        const response = await fetch("/kontoo/entries/assetinfo", {
            method: "POST",
            body: JSON.stringify({
                assetId: assetId,
                date: date
            }),
            headers: {
                "Content-Type": "application/json"
            }
        });
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const data = await response.json();
        if (data.status !== "OK") {
            showAssetInfo(false);
            return;
        }
        showAssetInfo(true, data.innerHTML);
    }
    catch (error) {
        console.error("Error fetching asset info:", error);
    }
}

function assetIdChange(assetId) {
    if (!assetId) {
        return;
    }
    const assetList = document.getElementById("AssetList");
    // All IDs are prefixed with OptionID_ b/c numeric IDs otherwise don't yield a valid DOM ID.
    const asset = assetList.options["OptionID_"+assetId];
    if (asset == null) {
        // Unknown asset: enable all types
        document.querySelectorAll("#TypeList option").forEach((opt) => {
            opt.disabled = false
        });
        document.querySelector("#AssetName").textContent = "Unknown asset";
        return;
    }
    document.querySelector("#AssetName").textContent = asset.textContent;
    const entryTypes = asset.dataset.entryTypes.split(" ");
    document.querySelectorAll("#TypeList option").forEach((opt) => {
        opt.disabled = !entryTypes.includes(opt.value);
    });
    // Move focus to the next input field.
    document.querySelector("#Type").focus();
    fetchAssetInfo(assetId);
}

function showAssetInfo(show = true, innerHTML = null) {
    const assetInfo = document.querySelector("#asset-info");
    if (show) {
        assetInfo.innerHTML = innerHTML;
        assetInfo.classList.remove("hidden");
    } else {
        assetInfo.classList.add("hidden")
    }
}

function entryTypeChange(typ) {
    if (typ === "ExchangeRate") {
        showAssetInfo(false);
        showFields(["QuoteCurrency", "Price"]);
    } else if (typ === "AccountBalance" || typ === "AccountDebit" || typ === "AccountCredit") {
        showFields(["AssetID", "Value"]);
    } else if (typ === "AssetHolding") {
        showFields(["AssetID", "Value", "Quantity", "Price"]);
    } else if (typ === "InterestPayment" || typ == "DividendPayment") {
        showFields(["AssetID", "Value"]);
    } else if (typ === "AssetPrice") {
        showFields(["AssetID", "Price"]);
    } else if (typ === "AssetMaturity") {
        showFields(["AssetID", "Value"]);
    } else {
        showFields(["AssetID", "Value", "Quantity", "Price", "Cost"]);
    }
}

function showFields(fieldNames) {
    const allFieldNames = [
        "AssetID", "Value", "Quantity", "Price", "Cost", "QuoteCurrency"
    ];
    for (const fieldName of allFieldNames) {
        if (fieldNames.includes(fieldName)) {
            document.querySelector(`#${fieldName}Field`).classList.remove("hidden");
        } else {
            document.querySelector(`#${fieldName}Field`).classList.add("hidden");
        }
    }
}

async function submitForm(event) {
    event.preventDefault(); // Prevent the default form submission
    const formData = new FormData(this);
    const clickedButton = event.submitter;
    const entry = {}
    formData.forEach((value, key) => {
        if (!value) {
            return;
        }
        if (key === "SequenceNum") {
            entry[key] = parseInt(value);
        } else {
            entry[key] = value;
        }
    })
    try {
        const update = formData.has("SequenceNum");
        const response = await fetch(this.action, {
            method: "POST",
            body: JSON.stringify({
                updateExisting: update,
                entry: entry
            }),
            headers: {
                "Content-Type": "application/json"
            }
        });
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const data = await response.json();
        if (data.status === "OK") {
            const tm = new Date().toLocaleTimeString('en-GB');
            callout(`${tm} - ${update ? "Updated" : "Added"} ledger entry with sequence number ${data.sequenceNum}.`);
            // this.reset();
            fetchAssetInfo();
        } else {
            calloutStatus(data.status, data.error);
        }
    }
    catch (error) {
        console.error("Error on submit:", error);
    }
}

export function init() {
    const entryForm = document.querySelector("#entry-form");
    entryForm.addEventListener("submit", submitForm);
    document.querySelector("#AssetID").addEventListener("input", function (event) {
        inputAutocomplete(event, assetIdChange);
    });
    document.querySelector("#Type").addEventListener("input", function (event) {
        inputAutocomplete(event, entryTypeChange);
    });
    const quoteCurrency = document.querySelector("#QuoteCurrency")
    if (quoteCurrency) {
        // Might be missing, if ledger only has entries for base currency.
        quoteCurrency.addEventListener("change", function (event) {
            const ccy = event.target.value;
            const label = document.querySelector("#ExchangeRateLabel")
            label.textContent = label.dataset.baseCurrency + "/" + ccy;
        });
    }
    // Adjust UI to preselected AssetID:
    assetIdChange(document.querySelector("#AssetID").value);
    // Update asset info on date change, too:
    document.querySelector("#ValueDate")._flatpickr.config.onValueUpdate.push(() => fetchAssetInfo());

}
