import { calloutError, hideCallout } from './common';

export function init() {
    const form = document.getElementById('irr-form');
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
