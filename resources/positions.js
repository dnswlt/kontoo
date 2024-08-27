import { registerDropdown, registerContextMenu } from "./common";

function followUrl(item) {
    window.location.href = item.dataset.url;
}

export function init() {
    registerDropdown("months-dropdown", followUrl);
    registerDropdown("years-dropdown", followUrl);
    document.querySelectorAll(".contextmenu.entry-actions").forEach((td) => {
        registerContextMenu(td, followUrl);
    });
}
