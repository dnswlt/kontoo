import { calloutError, calloutStatus, hideCallout } from "./common";

export function init() {
    // Send calculate IRR JSON request to backend on click
    document.querySelector("#calculate-irr").addEventListener("click", async function () {
        const nominalValue = document.querySelector("#NominalValue").value;
        const purchasePrice = document.querySelector("#PurchasePrice").value;
        const cost = document.querySelector("#Cost").value;
        const purchaseDate = document.querySelector("#PurchaseDate").value;
        const maturityDate = document.querySelector("#MaturityDate").value;
        const interestRate = document.querySelector("#InterestRate").value;
        try {
            const payload = {
                "nominalValue": nominalValue,
                "purchasePrice": purchasePrice,
                "cost": cost,
                "purchaseDate": purchaseDate,
                "maturityDate": maturityDate,
                "interestRate": interestRate,
                "accruedInterest": false,
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
            hideCallout();
            document.querySelector("#IRR").value = data.irrFormatted;
        }
        catch (error) {
            console.error("Error on submit:", error);
        }
    });
}
