/**
 * Compose image gallery — the dedicated image area on the compose page.
 *
 * Drag & drop or multi-select upload (POST /compose/image/:slug, one request
 * per file, sequential to preserve order). The server stores a compressed
 * full-size image plus a square center-crop thumbnail; this module tracks the
 * returned full URLs in the hidden `images` input (newline-separated) that
 * the form submits with the article, renders tiles with derived thumb URLs,
 * and removes tiles via DELETE /compose/image/:slug/:filename.
 *
 * Upload requires a saved draft (slug) — same discipline as content/banner
 * uploads. Mirrors imaging.ThumbName for thumbnail derivation.
 */

import { authenticatedFetch } from './auth-fetch.js';
import { openLightbox } from './lightbox.js';

/** Derive the square thumbnail URL for an uploads URL (see imaging.ThumbName). */
export function thumbUrl(url) {
    const slash = url.lastIndexOf('/');
    if (slash < 0) return url;
    const dir = url.slice(0, slash + 1);
    const name = url.slice(slash + 1);
    const dot = name.lastIndexOf('.');
    if (dot < 0) return url;
    const base = name.slice(0, dot);
    const ext = name.slice(dot).toLowerCase();
    if (ext === '.png') return dir + base + '-thumb.png';
    if (ext === '.jpg' || ext === '.jpeg' || ext === '.webp' || ext === '.gif') return dir + base + '-thumb.jpg';
    return url;
}

function formatFileSize(bytes) {
    if (bytes >= 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(0) + ' MB';
    if (bytes >= 1024) return (bytes / 1024).toFixed(0) + ' KB';
    return bytes + ' bytes';
}

/**
 * Create the gallery controller. Returns { refresh } — compose.js calls
 * refresh() from updateUploadButton so the add affordance tracks slug state.
 */
export function createComposeImages({ signal, form, getSlug }) {
    const field = document.querySelector('[data-gallery-field]');
    if (!field) return { refresh() {} };

    const dropZone = field.querySelector('[data-gallery-drop]');
    const grid = field.querySelector('[data-gallery-grid]');
    const addBtn = field.querySelector('[data-gallery-add]');
    const fileInput = field.querySelector('[data-gallery-file-input]');
    const statusEl = field.querySelector('[data-gallery-status]');
    const hiddenInput = field.querySelector('[data-images-input]');

    const maxFileSize = parseInt(form?.dataset.maxUpload || '10485760', 10);

    function getImages() {
        return hiddenInput.value.split('\n').map((s) => s.trim()).filter(Boolean);
    }

    function setImages(urls) {
        hiddenInput.value = urls.join('\n');
        render();
    }

    function setStatus(msg, isError) {
        if (!statusEl) return;
        statusEl.textContent = msg;
        statusEl.className = 'compose-gallery-status' + (isError ? ' upload-error' : msg ? ' upload-success' : '');
    }

    function render() {
        grid.textContent = '';
        for (const url of getImages()) {
            const tile = document.createElement('div');
            tile.className = 'compose-gallery-tile';

            const img = document.createElement('img');
            img.src = thumbUrl(url);
            img.alt = '';
            img.loading = 'lazy';
            img.addEventListener('click', () => openLightbox(getImages(), getImages().indexOf(url)));
            tile.appendChild(img);

            const remove = document.createElement('button');
            remove.type = 'button';
            remove.className = 'compose-gallery-remove';
            remove.setAttribute('aria-label', 'Remove image');
            remove.textContent = '\u00d7';
            remove.addEventListener('click', () => removeImage(url, remove), { signal });
            tile.appendChild(remove);

            grid.appendChild(tile);
        }
    }

    function filenameOf(url) {
        const i = url.lastIndexOf('/');
        return i >= 0 ? url.slice(i + 1) : url;
    }

    async function removeImage(url, btn) {
        const slug = getSlug();
        if (!slug) return;
        btn.disabled = true;
        try {
            const res = await authenticatedFetch(
                `/compose/image/${encodeURIComponent(slug)}/${encodeURIComponent(filenameOf(url))}`,
                { method: 'DELETE' }
            );
            if (!res.ok && res.status !== 404) {
                throw new Error('Remove failed');
            }
            setImages(getImages().filter((u) => u !== url));
            setStatus('Image removed', false);
        } catch (err) {
            setStatus(err.message || 'Remove failed', true);
        } finally {
            btn.disabled = false;
        }
    }

    function uploadNext(slug, files, index) {
        if (index >= files.length) return;
        const file = files[index];

        if (!file.type.startsWith('image/')) {
            setStatus(`Skipped ${file.name}: not an image`, true);
            uploadNext(slug, files, index + 1);
            return;
        }
        if (file.size > maxFileSize) {
            setStatus(`Skipped ${file.name}: exceeds ${formatFileSize(maxFileSize)} limit`, true);
            uploadNext(slug, files, index + 1);
            return;
        }

        setStatus(`Uploading ${index + 1}/${files.length}: ${file.name}`, false);
        const formData = new FormData();
        formData.append('file', file);

        authenticatedFetch(`/compose/image/${encodeURIComponent(slug)}`, {
            method: 'POST',
            body: formData,
        })
            .then(async (res) => {
                const data = await res.json().catch(() => ({}));
                if (!res.ok) throw new Error(data.error || 'Upload failed');
                setImages([...getImages(), data.url]);
                uploadNext(slug, files, index + 1);
            })
            .catch((err) => {
                setStatus(err.message || 'Upload failed', true);
            });
    }

    function uploadFiles(fileList) {
        const files = Array.from(fileList || []);
        if (files.length === 0) return;
        const slug = getSlug();
        if (!slug) {
            setStatus('Save as draft first to upload images', false);
            return;
        }
        uploadNext(slug, files, 0);
    }

    // File picker
    addBtn.addEventListener('click', () => fileInput.click(), { signal });
    fileInput.addEventListener('change', () => {
        uploadFiles(fileInput.files);
        fileInput.value = '';
    }, { signal });

    // Drag & drop
    dropZone.addEventListener('dragover', (e) => {
        e.preventDefault();
        if (getSlug()) dropZone.classList.add('compose-gallery-dragover');
    }, { signal });
    dropZone.addEventListener('dragleave', () => {
        dropZone.classList.remove('compose-gallery-dragover');
    }, { signal });
    dropZone.addEventListener('drop', (e) => {
        e.preventDefault();
        dropZone.classList.remove('compose-gallery-dragover');
        uploadFiles(e.dataTransfer.files);
    }, { signal });

    render();

    return {
        refresh() {
            const slug = getSlug();
            addBtn.disabled = !slug;
            if (!slug) {
                setStatus('Save as draft first to upload images', false);
            }
        },
    };
}
