/**
 * Lightbox — modal viewer for gallery images and markdown body images.
 *
 * openLightbox(images, startIndex) renders a fixed overlay with prev/next
 * navigation, a counter, and keyboard support (Esc closes, arrows navigate).
 * initImageGalleries() wires every [data-gallery] container: tiles carry
 * data-gallery-index; the container's data-full-urls attribute (JSON array,
 * rendered server-side) holds the full-size URLs including those beyond the
 * nine-grid cutoff.
 * initContentImages() wires images embedded in a markdown body: the renderer
 * wraps each local upload in <a class="content-image-link" href="full">, whose
 * inline <img> is the downscaled copy. Navigation stays within the enclosing
 * post so the feed's many cards never bleed into one another.
 */

let active = null;
let lastFocus = null;

// Containers that scope one post's in-content images. The first match walking
// up from the clicked link wins; falling back to the document keeps standalone
// pages (about, previews) working.
const CONTENT_IMAGE_GROUP_SELECTOR =
    '.article-content, .feed-card-content, .thought-content, .compose-preview-content';

export function openLightbox(images, startIndex) {
    closeLightbox();
    if (!Array.isArray(images) || images.length === 0) return;

    lastFocus = document.activeElement;

    const overlay = document.createElement('div');
    overlay.className = 'lightbox-overlay';
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.setAttribute('aria-label', 'Image viewer');

    const figure = document.createElement('div');
    figure.className = 'lightbox-figure';
    const img = document.createElement('img');
    img.alt = '';
    figure.appendChild(img);

    const counter = document.createElement('span');
    counter.className = 'lightbox-counter';

    const closeBtn = document.createElement('button');
    closeBtn.type = 'button';
    closeBtn.className = 'lightbox-close';
    closeBtn.setAttribute('aria-label', 'Close viewer');
    closeBtn.textContent = '\u00d7';

    const prevBtn = document.createElement('button');
    prevBtn.type = 'button';
    prevBtn.className = 'lightbox-nav lightbox-nav-prev';
    prevBtn.setAttribute('aria-label', 'Previous image');
    prevBtn.textContent = '\u2039';

    const nextBtn = document.createElement('button');
    nextBtn.type = 'button';
    nextBtn.className = 'lightbox-nav lightbox-nav-next';
    nextBtn.setAttribute('aria-label', 'Next image');
    nextBtn.textContent = '\u203a';

    overlay.appendChild(figure);
    overlay.appendChild(counter);
    overlay.appendChild(closeBtn);
    overlay.appendChild(prevBtn);
    overlay.appendChild(nextBtn);

    let index = Math.min(Math.max(startIndex || 0, 0), images.length - 1);

    function render() {
        img.src = images[index];
        counter.textContent = (index + 1) + ' / ' + images.length;
        prevBtn.hidden = images.length <= 1;
        nextBtn.hidden = images.length <= 1;
    }

    function step(delta) {
        index = (index + delta + images.length) % images.length;
        render();
    }

    function onKey(e) {
        if (e.key === 'Escape') closeLightbox();
        else if (e.key === 'ArrowLeft') step(-1);
        else if (e.key === 'ArrowRight') step(1);
    }

    function onCloseClick(e) {
        if (e.target === overlay || e.target === figure) closeLightbox();
    }

    closeBtn.addEventListener('click', closeLightbox);
    prevBtn.addEventListener('click', () => step(-1));
    nextBtn.addEventListener('click', () => step(1));
    overlay.addEventListener('click', onCloseClick);
    document.addEventListener('keydown', onKey);

    document.body.style.overflow = 'hidden';
    document.body.appendChild(overlay);
    render();
    closeBtn.focus();

    active = {
        overlay,
        cleanup() {
            document.removeEventListener('keydown', onKey);
            overlay.remove();
            document.body.style.overflow = '';
            if (lastFocus && typeof lastFocus.focus === 'function') lastFocus.focus();
        },
    };
}

export function closeLightbox() {
    if (active) {
        active.cleanup();
        active = null;
    }
}

/**
 * Wire click handlers on images embedded in markdown bodies. Safe to re-run
 * after SPA content swaps — each link is initialized at most once.
 */
export function initContentImages() {
    document.querySelectorAll('a.content-image-link').forEach((link) => {
        if (link.dataset.contentImageReady) return;
        link.dataset.contentImageReady = 'true';

        link.addEventListener('click', (e) => {
            // Modified clicks (open in new tab, save image, etc.) keep the
            // native anchor behaviour — href is the full-size image.
            if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;

            const group = link.closest(CONTENT_IMAGE_GROUP_SELECTOR) || document;
            const links = Array.from(group.querySelectorAll('a.content-image-link'));
            const urls = links.map((a) => a.href);
            if (urls.length === 0) return;

            e.preventDefault();
            openLightbox(urls, Math.max(links.indexOf(link), 0));
        });
    });
}

/**
 * Wire click handlers on all [data-gallery] containers. Safe to re-run after
 * SPA content swaps — each container is initialized at most once.
 */
export function initImageGalleries() {
    document.querySelectorAll('[data-gallery]').forEach((container) => {
        if (container.dataset.galleryReady) return;
        container.dataset.galleryReady = 'true';

        let urls = [];
        // data-full-urls is a comma-joined URL list rendered by the
        // image-gallery template (upload URLs contain no commas).
        urls = (container.dataset.fullUrls || '').split(',').filter(Boolean);

        container.querySelectorAll('[data-gallery-index]').forEach((tile) => {
            tile.addEventListener('click', () => {
                const i = parseInt(tile.dataset.galleryIndex, 10);
                if (urls.length > 0) openLightbox(urls, Number.isNaN(i) ? 0 : i);
            });
        });
    });
}
