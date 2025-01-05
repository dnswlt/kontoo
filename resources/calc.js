import { calloutError, calloutStatus } from "./common";

export function init() {
    // Send calculate IRR JSON request to backend on click
    document.querySelector("#calculate-irr").addEventListener("click", async function () {
        const purchasePrice = document.querySelector("#PurchasePrice").value;
        const purchaseDate = document.querySelector("#PurchaseDate").value;
        const maturityDate = document.querySelector("#MaturityDate").value;
        const interestRate = document.querySelector("#InterestRate").value;
        const interestDate = document.querySelector("#InterestDate").value;
        try {
            const payload = {
                "purchasePrice": purchasePrice,
                "purchaseDate": purchaseDate,
                "maturityDate": maturityDate,
                "interestRate": interestRate,
            }
            if (interestDate.trim() !== "") {
                payload["interestDate"] = interestDate;
            }
            const response = await fetch("/kontoo/calculate", {
                method: "POST",
                body: JSON.stringify(payload),
                headers: {
                    "Content-Type": "application/json"
                }
            });
            if (!response.ok) {
                const errorMsg = await response.text();
                calloutError(`that did not go so well: ${errorMsg}`);
                return;
            }
            const data = await response.json();
            if (data.status !== "OK") {
                calloutStatus(data.status, data.error);
                return;
            }
            // Display result.
            document.querySelector("#IRR").value = data.irrFormatted;
        }
        catch (error) {
            console.error("Error on submit:", error);
        }
    });
}
