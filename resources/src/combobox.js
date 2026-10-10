// Editable choices in a native popover. The datalist remains the source of
// values and metadata, and provides a fallback before this module initializes.
// The popover is manual: focus never leaves the input while it is open, so
// focusout dismisses it, and clicks on the input don't light-dismiss it.
export function initCombobox(input, { autocomplete = false, assetNames = false } = {}) {
    const source = input.list;
    const wrapper = input.closest('.entry-combobox');
    const toggle = wrapper.querySelector('.entry-combobox-toggle');
    const popup = wrapper.querySelector('[popover]');
    const list = popup.querySelector('[role="listbox"]');
    const empty = popup.querySelector('.entry-combobox-empty');
    let matches = [];
    let active = -1;
    let query = '';

    input.removeAttribute('list'); // Suppress the browser's competing datalist UI.
    input.setAttribute('role', 'combobox');
    input.setAttribute('aria-autocomplete', 'list');
    input.setAttribute('aria-controls', list.id);
    input.setAttribute('aria-expanded', 'false');

    const isOpen = () => popup.matches(':popover-open');
    const choices = () => [...source.options].filter(option => !option.disabled).map(option => ({
        value: option.value,
        label: option.textContent.trim() || option.value
    }));

    function setActive(index) {
        active = index;
        [...list.children].forEach((option, i) => {
            option.setAttribute('aria-selected', String(i === active));
            option.classList.toggle('is-current', matches[i].value === input.value);
        });
        if (active >= 0 && list.children[active]) {
            input.setAttribute('aria-activedescendant', list.children[active].id);
            list.children[active].scrollIntoView({ block: 'nearest' });
        } else {
            input.removeAttribute('aria-activedescendant');
        }
    }

    function position() {
        if (!isOpen()) return;
        const anchor = wrapper.getBoundingClientRect();
        const margin = 12;
        popup.style.width = `${Math.min(Math.max(anchor.width, assetNames ? 380 : anchor.width), window.innerWidth - margin * 2)}px`;
        popup.style.maxHeight = '320px';
        const below = window.innerHeight - anchor.bottom - margin - 4;
        const above = anchor.top - margin - 4;
        const useBelow = below >= popup.offsetHeight || below >= above;
        popup.style.maxHeight = `${Math.max(60, Math.min(320, useBelow ? below : above))}px`;
        popup.style.left = `${Math.max(margin, Math.min(anchor.left, window.innerWidth - popup.offsetWidth - margin))}px`;
        popup.style.top = `${useBelow ? anchor.bottom + 4 : Math.max(margin, anchor.top - popup.offsetHeight - 4)}px`;
    }

    function render() {
        const term = query.trim().toLowerCase();
        matches = choices().filter(option => `${option.label} ${option.value}`.toLowerCase().includes(term));
        list.replaceChildren();
        matches.forEach((option, index) => {
            const row = document.createElement('div');
            row.id = `${list.id}-${index}`;
            row.className = 'entry-combobox-option';
            row.setAttribute('role', 'option');
            const label = document.createElement('span');
            label.textContent = option.label;
            row.append(label);
            if (assetNames) {
                const identifier = document.createElement('span');
                identifier.className = 'entry-combobox-identifier';
                identifier.textContent = option.value;
                row.append(identifier);
            }
            row.addEventListener('pointermove', () => setActive(index));
            row.addEventListener('click', () => choose(index));
            list.append(row);
        });
        empty.hidden = matches.length > 0;
        setActive(matches.findIndex(option => option.value === input.value));
        position();
    }

    function open(all = false) {
        if (input.disabled) return;
        if (all) query = '';
        render();
        if (!isOpen()) popup.showPopover();
        input.setAttribute('aria-expanded', 'true');
        position();
    }

    function close() {
        if (isOpen()) popup.hidePopover();
        input.setAttribute('aria-expanded', 'false');
        setActive(-1);
    }

    function choose(index) {
        if (!matches[index]) return;
        input.value = matches[index].value;
        input.dispatchEvent(new Event('input', { bubbles: true }));
        input.dispatchEvent(new Event('change', { bubbles: true }));
        close();
        input.focus();
    }

    input.addEventListener('input', event => {
        // Only typing opens the list; choose() dispatches untrusted events.
        if (!event.isTrusted) return;
        query = input.value;
        open();
        // Complete a unique match after 3 characters, as the old datalist UI did.
        // Backspace/Delete always edit freely, and completion never moves focus.
        if (autocomplete && !event.isComposing && !event.inputType?.startsWith('delete') &&
            query.length >= 3 && matches.length === 1) {
            input.value = matches[0].value;
            setActive(0);
        }
    });
    input.addEventListener('click', () => { if (!isOpen()) open(true); });
    input.addEventListener('keydown', event => {
        if (event.isComposing) return;
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault();
            const wasOpen = isOpen();
            if (!wasOpen) open(true);
            if (matches.length) {
                const next = active < 0 ? (event.key === 'ArrowDown' ? 0 : matches.length - 1)
                    : wasOpen ? (active + (event.key === 'ArrowDown' ? 1 : -1) + matches.length) % matches.length : active;
                setActive(next);
            }
        } else if (event.key === 'Enter' && isOpen()) {
            event.preventDefault();
            if (active >= 0) choose(active);
            else if (matches.length === 1) choose(0);
            else close();
        } else if (event.key === 'Escape' && isOpen()) {
            event.preventDefault();
            close();
        } else if (event.key === 'Tab') {
            if (isOpen()) {
                if (active >= 0) choose(active);
                else if (matches.length === 1) choose(0);
            }
            close();
        }
    });
    // Keep focus in the input when clicking the toggle, options or scrollbar.
    toggle.addEventListener('mousedown', event => event.preventDefault());
    popup.addEventListener('mousedown', event => event.preventDefault());
    toggle.addEventListener('click', () => {
        input.focus();
        if (isOpen()) close();
        else open(true);
    });
    input.addEventListener('blur', close);
    window.addEventListener('resize', position);
    window.addEventListener('scroll', position, { capture: true, passive: true });

    return { refresh: () => { if (isOpen()) render(); } };
}
