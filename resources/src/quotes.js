import { registerQuotesSubmit } from "./common";

export function init() {
    registerQuotesSubmit();
    document.getElementById("QuoteDate")._flatpickr.set("maxDate", "today");
}