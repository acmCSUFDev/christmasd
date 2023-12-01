const settingsDialog = document.getElementById("connect-settings");
const settingsForm = settingsDialog.querySelector("form");
const treeCanvas: HTMLCanvasElement = document.getElementById("tree");
const console: HTMLPreElement = document.getElementById("console");

settingsForm.addEventListener("submit", (event) => {
  const formData = new FormData(settingsForm);
  const server = formData.get("server") as string;
});
