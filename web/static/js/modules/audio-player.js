/**
 * Feed audio player — play/pause buttons on feed cards.
 *
 * Each card with music renders a [data-music-src] button; this module keeps
 * ONE module-level Audio element so at most a single track plays at a time —
 * starting another card's track pauses and resets the previous one. Page
 * navigations (SPA swaps) call stopFeedAudio() so playback never leaks
 * across pages; the article page uses the browser's native <audio controls>,
 * which needs no JS.
 */

let currentAudio = null;
let currentBtn = null;

const ICON_PLAY = '\u25b6';
const ICON_PAUSE = '\u23f8';

function resetCurrent() {
    if (currentAudio) {
        currentAudio.pause();
        currentAudio = null;
    }
    if (currentBtn) {
        currentBtn.textContent = ICON_PLAY;
        currentBtn.setAttribute('aria-label', 'Play music');
        currentBtn.classList.remove('feed-music-btn-playing');
        currentBtn = null;
    }
}

export function stopFeedAudio() {
    resetCurrent();
}

/**
 * Wire click handlers on all [data-music-src] feed buttons. Safe to re-run
 * after SPA content swaps — each button is initialized at most once.
 */
export function initFeedAudio() {
    document.querySelectorAll('[data-music-src]').forEach((btn) => {
        if (btn.dataset.musicReady) return;
        btn.dataset.musicReady = 'true';

        btn.addEventListener('click', () => {
            const src = btn.dataset.musicSrc;
            if (!src) return;

            // Same button toggles play/pause.
            if (btn === currentBtn) {
                resetCurrent();
                return;
            }

            // Different track: stop whatever is playing first.
            resetCurrent();

            const audio = new Audio(src);
            audio.addEventListener('ended', () => {
                if (currentAudio === audio) resetCurrent();
            });
            audio.play().then(() => {
                currentAudio = audio;
                currentBtn = btn;
                btn.textContent = ICON_PAUSE;
                btn.setAttribute('aria-label', 'Pause music');
                btn.classList.add('feed-music-btn-playing');
            }).catch(() => {
                // Load/play failed — leave the button in its rest state.
            });
        });
    });
}
