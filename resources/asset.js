import { callout, calloutStatus } from './common';

export function init() {
    console.log("asset.init");
    const entryForm = document.querySelector("#asset-form");
    entryForm.addEventListener("submit", async function (event) {
        event.preventDefault(); // Prevent the default form submission
        const formData = new FormData(this);
        const asset = {};
        formData.forEach((value, key) => {
            if (!value) {
                return;
            }
            if (key === "QuoteServiceSymbols") {
                asset[key] = {
                    "YF": value
                };
            } else {
                asset[key] = value;
            }
        })
        try {
            const response = await fetch("/kontoo/assets", {
                method: "POST",
                body: JSON.stringify(asset),
                headers: {
                    "Content-Type": "application/json"
                }
            });
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const data = await response.json();
            if (data.status === "OK") {
                callout(`Successfully added asset ${data.assetId}.`);
                this.reset();
            } else {
                calloutStatus(data.status, data.error);
            }
        }
        catch (error) {
            console.error("Error on submit:", error);
        }
    });
    document.querySelector("#Type").addEventListener("keydown", function (event) {
        if (event.key === "Backspace") {
            event.target.value = "";
            event.preventDefault();
        }
    });
    document.querySelector("#Currency").addEventListener("change", function (event) {
        const input = event.target;
        input.value = input.value.toUpperCase();
    });
    document.querySelector("#Interest").addEventListener("change", function (event) {
        const input = event.target;
        if (input.value && !input.value.endsWith("%")) {
            if (!isNaN(Number.parseFloat(input.value))) {
                input.value = input.value + "%";
            }
        }
    });
}