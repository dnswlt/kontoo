import { callout, calloutStatus } from "./common";


function inputEventForList(event, callback) {
    const input = event.target;
    if (event.inputType.startsWith("deleteContent")) {
        // Don't auto-complete if Backspace or Del were hit.
        return;
    }
    const token = input.value.toLowerCase();
    if (token.length < 3) {
        return;  // Require at least 3 characters before trying to match.
    }
    const datalistId = input.getAttribute('list');
    const options = document.querySelectorAll(`#${datalistId} option`);
    const assetIds = Array.from(options).map(opt => ({
        id: opt.value,
        searchText: (opt.value + " " + opt.textContent).toLowerCase()
    }));
    const matches = assetIds.filter(a => a.searchText.includes(token));
    if (matches.length != 1) {
        // No unique match
        return;
    }
    input.value = matches[0].id;
    callback(matches[0].id);
}

function assetIdChange(assetId) {
    if (!assetId) {
        // Empty assetId, do nothing.
        return;
    }
    const assetList = document.getElementById("AssetList");
    const asset = assetList.options[assetId];
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
}

function typeFieldChanged(typ) {
    if (typ === "ExchangeRate") {
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

export function init() {
    const entryForm = document.querySelector("#entry-form");
    entryForm.addEventListener("submit", async function (event) {
        event.preventDefault(); // Prevent the default form submission
        const formData = new FormData(this);
        const clickedButton = event.submitter;
        const entry = {}
        formData.forEach((value, key) => {
            if (value) {
                entry[key] = value;
            }
        })
        try {
            const response = await fetch("/kontoo/entries/add", {
                method: "POST",
                body: JSON.stringify(entry),
                headers: {
                    "Content-Type": "application/json"
                }
            });
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const data = await response.json();
            if (data.status === "OK") {
                callout(`Added ledger entry with sequence number ${data.sequenceNum}.`);
                this.reset();
            } else {
                calloutStatus(data.status, data.error);
            }
        }
        catch (error) {
            console.error("Error on submit:", error);
        }
    });
    document.querySelector("#AssetID").addEventListener("input", function (event) {
        inputEventForList(event, assetIdChange);
    });
    document.querySelector("#Type").addEventListener("input", function (event) {
        inputEventForList(event, typeFieldChanged);
    });
    // Adjust UI to preselected AssetID:
    assetIdChange(document.querySelector("#AssetID").value);
}
