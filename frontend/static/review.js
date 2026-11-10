const reviewForm = document.getElementById("review-form");

if (reviewForm) {
    const scoreInput = document.getElementById("review-score");
    const commentInput = document.getElementById("review-comment");
    const previewButton = document.getElementById("review-preview-button");
    const preview = document.getElementById("review-preview");

    reviewForm.addEventListener("submit", function (event) {
        event.preventDefault();
    });
    reviewForm.addEventListener("input", function () {
        preview.hidden = true;
    });

    for (const button of document.querySelectorAll(".score-preset")) {
        button.disabled = false;
        button.addEventListener("click", function () {
            scoreInput.value = button.dataset.score;
            preview.hidden = true;
        });
    }

    previewButton.disabled = false;
    previewButton.addEventListener("click", function () {
        if (!reviewForm.reportValidity()) return;
        const comment = commentInput.value.trim() || "Без комментария";
        preview.textContent = "Предпросмотр: " + scoreInput.value + " / " + scoreInput.max
            + "\n" + comment + "\nРезультат не опубликован и не сохранён.";
        preview.hidden = false;
    });
}
