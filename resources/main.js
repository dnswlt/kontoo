import flatpickr from 'flatpickr';
// Why on earth is it not documented properly on the flatpickr webpage,
// that you have to import the CSS, too?!?
// See https://github.com/flatpickr/flatpickr/issues/141
import 'flatpickr/dist/flatpickr.min.css';

//
// Global definitions
//

async function initEntryPage() {
    const entry = await import('./entry.js');
    entry.init();
}

async function initAssetPage() {
    const asset = await import('./asset.js');
    asset.init();
}

async function initLedgerPage() {
    const ledger = await import('./ledger.js');
    ledger.init();
}

// Validate that input contains a decimal number with an optional '%' at the end.
// (I.e., a string that can be JSON-parsed as Micros.)
const microsRegex = new RegExp("^([0-9]+(\\.[0-9]{0,6})?|\\.[0-9]{1,6})%?$");
function validateMicros(input) {
    input.addEventListener("change", function (event) {
        const input = event.target;
        let val = input.value;
        val = val.trim().replaceAll(" ", "").replaceAll("\'", "");
        if (val && !microsRegex.test(val)) {
            input.classList.add("invalid");
        } else {
            input.classList.remove("invalid");
            input.value = val;
        }
    });
}

//
// Main code
//

// Set up date pickers on any page.
document.querySelectorAll('.datepicker').forEach(flatpickr);
document.querySelectorAll("input.micros").forEach(validateMicros);
document.querySelectorAll("input.noblanks").forEach((input) => {
    input.addEventListener("change", function (event) {
        input.value = input.value.replaceAll(" ", "");
    });
});
document.querySelectorAll("input.trim").forEach((input) => {
    input.addEventListener("change", function (event) {
        input.value = input.value.trim();
    });
});

// Page-specific initialisation.
switch (document.body.id) {
    case "ledger-page":
        initLedgerPage();
        break;
    case "entry-page":
        initEntryPage();
        break;
    case "asset-page":
        initAssetPage();
        break;
    default:
        if (document.body.id) {
            console.error(`Page with body id ${document.body.id} not handled in main.js`);
        } else {
            console.log("Skipping page-specific initialisation for page without body id.");
        }
        break;
}
