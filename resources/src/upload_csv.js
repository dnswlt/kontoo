import { callout, calloutError, calloutStatus, registerQuotesSubmit } from './common';

async function uploadFiles(files) {
    const formData = new FormData();
    files.forEach(file => formData.append('file', file));
    try {
        const resp = await fetch('/kontoo/csv', {
            method: 'POST',
            body: formData,
        });
        if (!resp.ok) {
            const text = await resp.text();
            throw new Error(`Upload failed with status ${resp.status}: ${text}`);
        }
        const data = await resp.json();
        if (data.innerHTML) {
            const results = document.getElementById('results-section');
            results.classList.remove('hidden');
            results.innerHTML = data.innerHTML;
            registerQuotesSubmit();
        }
        if (data.status === 'OK') {
            callout(`Successfully read ${data.numEntries} rows.`);
        } else {
            calloutStatus(data.status, data.error);
        }
    } catch (error) {
        calloutError(`Error during upload: ${error}`);
    }
}

export function init() {
    const dropArea = document.getElementById('upload-drop-area');
    const fileInput = document.getElementById('upload-file');

    // Dropping a file anywhere else must not make the browser navigate to it.
    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
        document.body.addEventListener(eventName, event => event.preventDefault());
    });
    ['dragenter', 'dragover'].forEach(eventName => {
        dropArea.addEventListener(eventName, () => dropArea.classList.add('highlight'));
    });
    ['dragleave', 'drop'].forEach(eventName => {
        dropArea.addEventListener(eventName, () => dropArea.classList.remove('highlight'));
    });
    dropArea.addEventListener('drop', event => uploadFiles([...event.dataTransfer.files]));

    // The drop area is the file input's label: clicking it opens the file picker.
    fileInput.addEventListener('change', () => {
        uploadFiles([...fileInput.files]);
        fileInput.value = ''; // Allow choosing the same file again.
    });
}
