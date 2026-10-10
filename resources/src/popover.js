// Native popovers own opening, dismissal, and keyboard focus navigation.
// Only positioning needs scripting, so this also works without CSS anchors.
export function init() {
    document.querySelectorAll('[data-positioned-popover][popovertarget]').forEach(button => {
        const popover = document.getElementById(button.getAttribute('popovertarget'));
        const position = () => {
            if (!popover.matches(':popover-open')) return;
            const anchor = button.getBoundingClientRect();
            const margin = 12;
            const left = Math.max(margin, Math.min(anchor.left, window.innerWidth - popover.offsetWidth - margin));
            const below = anchor.bottom + 8;
            const top = below + popover.offsetHeight <= window.innerHeight - margin
                ? below
                : Math.max(margin, anchor.top - popover.offsetHeight - 8);
            popover.style.left = `${left}px`;
            popover.style.top = `${top}px`;
        };
        popover.addEventListener('toggle', position, true);
        window.addEventListener('resize', position);
        window.addEventListener('scroll', position, { capture: true, passive: true });
    });
}
