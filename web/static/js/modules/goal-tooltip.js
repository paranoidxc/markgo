let initialized = false;

export function init() {
    if (initialized) return;
    initialized = true;

    document.addEventListener('keydown', (event) => {
        if (event.key !== 'Escape') return;
        document.querySelectorAll('.goal-bar:hover, .goal-bar:focus-within').forEach((bar) => {
            bar.setAttribute('data-tooltip-dismissed', '');
        });
    });

    const resetTooltip = (event) => {
        const bar = event.target.closest?.('.goal-bar');
        if (bar && !bar.contains(event.relatedTarget)) {
            bar.removeAttribute('data-tooltip-dismissed');
        }
    };

    document.addEventListener('pointerover', resetTooltip);
    document.addEventListener('focusin', resetTooltip);
}
