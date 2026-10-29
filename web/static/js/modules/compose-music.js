/**
 * Compose music area — single-track audio field on the compose page.
 *
 * Uploads reuse the generic endpoint (POST /compose/upload/:slug) with an
 * accept="audio/*" file picker; the server-side form parser (ParseMusicInput)
 * re-validates extension + slug ownership on submit. One track per article:
 * adding a new track deletes the previous upload first (DELETE
 * /compose/upload/:slug/:filename) so no orphan audio accumulates.
 *
 * Like the image area, upload requires a saved draft (slug).
 */

import { authenticatedFetch } from './auth-fetch.js';

function filenameOf(url) {
    const i = url.lastIndexOf('/');
    return i >= 0 ? url.slice(i + 1) : url;
}

/**
 * Create the music controller. Returns { refresh } — compose.js calls
 * refresh() from updateUploadButton so the add affordance tracks slug state.
 */
export function createComposeMusic({ signal, getSlug }) {
    const field = document.querySelector('[data-music-field]');
    if (!field) return { refresh() {} };

    const trackEl = field.querySelector('[data-music-track]');
    const nameEl = field.querySelector('[data-music-name]');
    const previewBtn = field.querySelector('[data-music-preview]');
    const removeBtn = field.querySelector('[data-music-remove]');
    const addBtn = field.querySelector('[data-music-add]');
    const fileInput = field.querySelector('[data-music-file-input]');
    const statusEl = field.querySelector('[data-music-status]');
    const hiddenInput = field.querySelector('[data-music-input]');

    let previewAudio = null;

    function getTrack() {
        return hiddenInput.value.trim();
    }

    function setTrack(url) {
        hiddenInput.value = url || '';
        render();
    }

    function setStatus(msg, isError) {
        if (!statusEl) return;
        statusEl.textContent = msg;
        statusEl.className = 'compose-gallery-status' + (isError ? ' upload-error' : msg ? ' upload-success' : '');
    }

    function stopPreview() {
        if (previewAudio) {
            previewAudio.pause();
            previewAudio = null;
        }
        previewBtn.textContent = '\u25b6';
    }

    function render() {
        const url = getTrack();
        if (url) {
            nameEl.textContent = filenameOf(url);
            nameEl.title = url;
            trackEl.hidden = false;
            addBtn.hidden = true;
        } else {
            stopPreview();
            trackEl.hidden = true;
            addBtn.hidden = false;
        }
    }

    async function deleteFile(url) {
        if (!url) return;
        const slug = getSlug();
        if (!slug) return;
        try {
            const res = await authenticatedFetch(
                `/compose/upload/${encodeURIComponent(slug)}/${encodeURIComponent(filenameOf(url))}`,
                { method: 'DELETE' }
            );
            if (!res.ok && res.status !== 404) {
                throw new Error('Delete failed');
            }
        } catch {
            // Best-effort: the edit-time sweep (UnlinkRemovedMusic) still
            // removes replaced tracks on save.
        }
    }

    removeBtn.addEventListener('click', async () => {
        removeBtn.disabled = true;
        const url = getTrack();
        setTrack('');
        setStatus('Track removed', false);
        await deleteFile(url);
        removeBtn.disabled = false;
    }, { signal });

    previewBtn.addEventListener('click', () => {
        const url = getTrack();
        if (!url) return;
        if (previewAudio) {
            stopPreview();
            return;
        }
        previewAudio = new Audio(url);
        previewAudio.addEventListener('ended', stopPreview);
        previewAudio.play().catch(() => stopPreview());
        previewBtn.textContent = '\u23f8';
    }, { signal });

    addBtn.addEventListener('click', () => fileInput.click(), { signal });

    fileInput.addEventListener('change', async () => {
        const file = fileInput.files && fileInput.files[0];
        fileInput.value = '';
        if (!file) return;
        const slug = getSlug();
        if (!slug) {
            setStatus('Save as draft first to upload music', false);
            return;
        }
        if (!file.type.startsWith('audio/') && !/\.(mp3|m4a|aac|ogg|oga|opus|wav|flac|weba)$/i.test(file.name)) {
            setStatus('Please choose an audio file', true);
            return;
        }

        addBtn.disabled = true;
        removeBtn.disabled = true;
        setStatus(`Uploading ${file.name}`, false);
        try {
            // Replace semantics: drop the old track's file before uploading.
            await deleteFile(getTrack());

            const formData = new FormData();
            formData.append('file', file);
            const res = await authenticatedFetch(`/compose/upload/${encodeURIComponent(slug)}`, {
                method: 'POST',
                body: formData,
            });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) throw new Error(data.error || 'Upload failed');
            setTrack(data.url);
            setStatus('Track uploaded', false);
        } catch (err) {
            setStatus(err.message || 'Upload failed', true);
        } finally {
            addBtn.disabled = false;
            removeBtn.disabled = false;
        }
    }, { signal });

    render();

    return {
        refresh() {
            const slug = getSlug();
            addBtn.disabled = !slug;
            if (!slug) {
                setStatus('Save as draft first to upload music', false);
            }
        },
    };
}
