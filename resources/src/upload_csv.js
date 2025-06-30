import { callout, calloutError, calloutStatus, registerQuotesSubmit } from "./common";

export function init() {
    const dropArea = document.getElementById("upload-drop-area");

    // Prevent default drag behaviors
    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
        dropArea.addEventListener(eventName, preventDefaults, false);
        document.body.addEventListener(eventName, preventDefaults, false);
    });

    // Highlight drop area when item is dragged over it
    ['dragenter', 'dragover'].forEach(eventName => {
        dropArea.addEventListener(eventName, highlight, false);
    });

    ['dragleave', 'drop'].forEach(eventName => {
        dropArea.addEventListener(eventName, unhighlight, false);
    });

    // Handle dropped files
    dropArea.addEventListener('drop', handleDrop, false);

    async function uploadFiles(files) {
        const formData = new FormData();
        files.forEach(file => formData.append("file", file));
        try {
            const resp = await fetch("/kontoo/csv", {
                method: "POST",
                body: formData,
            });
            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(`Upload failed with status ${resp.status}: ${text}`);
            }
            const data = await resp.json();
            handleUploadReponse(data);
        }
        catch (error) {
            calloutError(`Error during upload: ${error}`);
        }
    }

    function handleUploadReponse(data) {
        if (data.innerHTML) {
            const div = document.getElementById("results-section");
            div.classList.remove("hidden");
            div.innerHTML = data.innerHTML;
            registerQuotesSubmit();
        }
        if (data.status === "OK") {
            callout(`Successfully read ${data.numEntries} rows.`);
        } else {
            calloutStatus(data.status, data.error);
        }
    }

    function preventDefaults(e) {
        e.preventDefault();
        e.stopPropagation();
    }

    function highlight(e) {
        dropArea.classList.add('highlight');
    }

    function unhighlight(e) {
        dropArea.classList.remove('highlight');
    }

    function handleDrop(e) {
        const files = [...e.dataTransfer.files];
        uploadFiles(files);
    }
}