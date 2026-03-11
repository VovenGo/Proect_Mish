const canvas = document.getElementById('sketch');
const ctx = canvas.getContext('2d');
const color = document.getElementById('color');
const size = document.getElementById('size');
const clearBtn = document.getElementById('clear');
const form = document.getElementById('render-form');
const resultImage = document.getElementById('result-image');
const resultEmpty = document.getElementById('result-empty');
const resultJSON = document.getElementById('result-json');
const jobMeta = document.getElementById('job-meta');

ctx.fillStyle = '#10111a';
ctx.fillRect(0, 0, canvas.width, canvas.height);
ctx.lineCap = 'round';
ctx.lineJoin = 'round';

let drawing = false;

function position(e) {
  const rect = canvas.getBoundingClientRect();
  const point = e.touches ? e.touches[0] : e;
  return {
    x: (point.clientX - rect.left) * (canvas.width / rect.width),
    y: (point.clientY - rect.top) * (canvas.height / rect.height)
  };
}

function start(e) {
  drawing = true;
  const p = position(e);
  ctx.beginPath();
  ctx.moveTo(p.x, p.y);
  e.preventDefault();
}

function move(e) {
  if (!drawing) return;
  const p = position(e);
  ctx.strokeStyle = color.value;
  ctx.lineWidth = Number(size.value);
  ctx.lineTo(p.x, p.y);
  ctx.stroke();
  e.preventDefault();
}

function stop() {
  drawing = false;
}

['mousedown', 'touchstart'].forEach(ev => canvas.addEventListener(ev, start, {passive: false}));
['mousemove', 'touchmove'].forEach(ev => canvas.addEventListener(ev, move, {passive: false}));
['mouseup', 'mouseleave', 'touchend'].forEach(ev => canvas.addEventListener(ev, stop));

clearBtn.addEventListener('click', () => {
  ctx.fillStyle = '#10111a';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
});

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  const fd = new FormData(form);
  fd.set('sketchDataUrl', canvas.toDataURL('image/png'));

  const submit = form.querySelector('button[type="submit"]');
  submit.disabled = true;
  submit.textContent = 'Шаманю...';

  try {
    const res = await fetch('/api/render', { method: 'POST', body: fd });
    if (!res.ok) {
      throw new Error(await res.text());
    }
    const data = await res.json();
    resultImage.src = data.outputUrl;
    resultImage.hidden = false;
    resultEmpty.hidden = true;
    resultJSON.hidden = false;
    resultJSON.textContent = JSON.stringify(data, null, 2);
    jobMeta.textContent = `job ${data.id} · ${data.provider}`;
  } catch (err) {
    alert(err.message || String(err));
  } finally {
    submit.disabled = false;
    submit.textContent = 'Зашаманить картинку';
  }
});
