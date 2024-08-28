import { callout, calloutStatus } from "./common";

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
            const response = await fetch("/kontoo/entries", {
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
    function assetIdChanged(assetId) {
        const assets = document.querySelectorAll("#AssetList option");
        let asset = null;
        for (const a of assets) {
            if (a.value === assetId) {
                asset = a;
                break;
            }
        }
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
    }
    document.querySelector("#AssetID").addEventListener("input", function (event) {
        assetIdChanged(event.target.value);
    });
    document.querySelector("#Type").addEventListener("keydown", function (event) {
        if (event.key === "Backspace") {
            event.target.value = "";
            event.preventDefault();
        }
    });
    document.querySelector("#Type").addEventListener("change", function (event) {
        const typ = event.target.value;
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
        } else {
            showFields(["AssetID", "Value", "Quantity", "Price", "Cost"]);
        }
    });
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
    // Adjust UI to preselected AssetID:
    assetIdChanged(document.querySelector("#AssetID").value);
}
