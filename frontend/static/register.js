const registrationForm = document.getElementById("registration-form");
const registrationButton = document.getElementById("registration-check");
const registrationName = document.getElementById("registration-name");
const registrationPassword = document.getElementById("registration-password");
const registrationResult = document.getElementById("registration-result");

registrationForm.addEventListener("submit", function (event) {
    event.preventDefault();
});
registrationForm.addEventListener("input", function () {
    registrationResult.hidden = true;
    registrationName.setCustomValidity("");
});
registrationButton.disabled = false;
registrationButton.addEventListener("click", function () {
    registrationName.setCustomValidity(registrationName.value.trim() ? "" : "Введите имя");
    if (!registrationForm.reportValidity()) return;
    registrationPassword.value = "";
    registrationResult.textContent = "Поля заполнены корректно. Это предпросмотр: аккаунт не создан, пароль не отправлен и очищен.";
    registrationResult.hidden = false;
});
