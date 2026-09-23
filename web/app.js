const form = document.querySelector('#search');
const submit = document.querySelector('#submit');
const cards = document.querySelector('#cards');
const error = document.querySelector('#error');
const summary = document.querySelector('#summary');
const reasons = document.querySelector('#reasons');
const alternatives = document.querySelector('#alternatives');
const alternativeCards = document.querySelector('#alternative-cards');
const money = value => new Intl.NumberFormat('ru-RU').format(value);
const el = (tag, cls, text) => { const node = document.createElement(tag); if (cls) node.className = cls; if (text !== undefined) node.textContent = text; return node; };
let ready = false;
let inFlight = false;

async function init() {
  try {
    const response = await fetch('/api/options');
    if (!response.ok) throw new Error('Не удалось загрузить каталог. Обновите страницу.');
    const options = await response.json();
    for (const [field, list] of Object.entries({city:options.cities, category:options.categories, format:options.formats, language:options.languages})) {
      for (const value of list) document.getElementById(field).add(new Option(value, value));
    }
    for (const value of options.cities) document.getElementById('bundle-city').add(new Option(value, value));
    for (const value of options.formats) document.getElementById('bundle-format').add(new Option(value, value));
    for (const value of options.languages) document.getElementById('bundle-language').add(new Option(value, value));
    const categoryBox = document.querySelector('#bundle-categories');
    for (const value of options.categories) {
      const label = el('label','category-choice');
      const input = document.createElement('input'); input.type = 'checkbox'; input.name = 'category'; input.value = value;
      label.append(input, document.createTextNode(value)); categoryBox.append(label);
    }
    form.elements.city.value = 'Алматы'; form.elements.category.value = 'Ведущий'; form.elements.format.value = 'корпоратив';
    document.querySelector('#bundle-city').value = 'Алматы'; document.querySelector('#bundle-format').value = 'свадьба';
    document.querySelectorAll('#bundle-categories input').forEach(input => { input.checked = ['Ведущий','Фотограф','Танцевальный коллектив'].includes(input.value); });
    renderBundleDurations();
    ready = true; submit.disabled = false;
  } catch (e) { error.hidden = false; error.textContent = e.message; }
}

function renderCard(item, q) {
  const c = item.contractor;
  const article = el('article', 'card');
  const top = el('div', 'card-top');
  top.append(el('div', 'avatar', c.name.split(' ').slice(0,2).map(w=>w[0]).join('')));
  const identity = el('div', 'identity'); identity.append(el('h3','',c.name),el('div','meta',`${q.category} · ${c.city}`));
  const price = el('div','price',`от ${money(c.price_from_kzt)} ₸`); price.append(el('small','','за мероприятие'));
  top.append(identity,price); article.append(top);
  const tags = el('div','tags');
  for (const text of ['Свободен по календарю', c.languages.join(' · '), c.max_hours === null ? 'Без ограничения присутствия' : `До ${c.max_hours} ч`]) tags.append(el('span','tag',text));
  if(c.synthetic) tags.append(el('span','tag warning','Синтетический профиль организаторов'));
  if(c.price_imputed) tags.append(el('span','tag warning','Цена заполнена организаторами'));
  if(c.city_imputed) tags.append(el('span','tag warning','Город заполнен организаторами'));
  article.append(tags);
  const why = el('div','explanation'); why.append(el('strong','','Почему в подборке'),document.createTextNode(item.explanation)); article.append(why);
  const details = el('details','profile'); details.append(el('summary','','Описание из каталога'),el('p','',c.description)); article.append(details);
  return article;
}

function renderAlternative(item, q) {
  const box = el('article', 'alternative');
  const title = item.kind === 'over_budget' ? 'Свободен в вашу дату, но дороже' : 'Тот же подрядчик в другую дату';
  box.append(el('h4','', title), el('p','', `${item.contractor.name} · от ${money(item.contractor.price_from_kzt)} ₸`), el('p','',item.message));
  if (item.evidence?.length) box.append(el('p','',`В описании: «${item.evidence[0]}»`));
  const button = el('button','',item.kind === 'over_budget' ? 'Поставить этот бюджет' : 'Выбрать эту дату');
  button.type = 'button';
  button.addEventListener('click', () => {
    if(item.kind === 'over_budget') form.elements.budget.value = item.contractor.price_from_kzt;
    else form.elements.date.value = item.date;
    form.requestSubmit();
  });
  box.append(button); return box;
}

form.addEventListener('submit', async event => {
  event.preventDefault(); if (!ready || inFlight) return;
  inFlight = true; submit.disabled = true; submit.textContent = 'Подбираем…'; error.hidden = true;
  cards.replaceChildren(); reasons.hidden = true; alternatives.hidden = true; document.querySelector('#count').textContent = 'Поиск'; summary.textContent = 'Проверяем условия и календарь…';
  const query = Object.fromEntries(new FormData(form));
  try {
    const response = await fetch('/api/recommend?' + new URLSearchParams(query));
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || 'Не удалось выполнить подбор.');
    document.querySelector('#count').textContent = `${result.cards.length} из ${result.eligible_count}`;
    if (result.status === 'matched') {
      summary.textContent = `${query.city} · ${query.date.split('-').reverse().join('.')} · ${query.format}. Подходит ${result.eligible_count}; показываем ${result.cards.length} по смысловому соответствию и цене.`;
      cards.replaceChildren(...result.cards.map(item => renderCard(item,query)));
    } else {
      const absent = result.status === 'category_absent';
      summary.textContent = absent ? 'Такой категории в городе пока нет.' : 'Подрядчики есть, но не проходят все условия.';
      const empty = el('div','empty'); empty.append(el('div','empty-symbol','↗'),el('h3','',absent ? 'Попробуйте другой город' : 'Давайте изменим условия'),el('p','',absent ? 'В каталоге нет этой комбинации города и категории. Выберите другую категорию или город.' : 'Посмотрите причины ниже. Попробуйте другую дату, бюджет или дополнительные условия.'));
      cards.replaceChildren(empty);
    }
    if (result.alternatives?.length) { alternatives.hidden = false; alternativeCards.replaceChildren(...result.alternatives.map(item => renderAlternative(item, query))); }
    const labels = {busy:'Заняты на выбранную дату',budget:'Начальная цена выше бюджета',format:'Не работают с этим форматом',language:'Не указан выбранный язык',duration:'Не хватает часов на площадке'};
    const failures = Object.entries(result.rejection_counts).filter(([,n])=>n>0);
    reasons.replaceChildren();
    if(failures.length) {
      reasons.hidden = false; reasons.append(el('strong','','Почему другие не попали в подборку'));
      const list = el('ul'); failures.forEach(([key,n])=>list.append(el('li','',`${labels[key]}: ${n}`))); reasons.append(list,el('p','','Один профиль может не пройти сразу несколько условий.'));
    } else if(result.status==='matched' && result.eligible_count<3) {
      reasons.hidden = false; reasons.append(el('p','',`В выбранном городе и категории всего ${result.eligible_count} профилей. Все они показаны.`));
    }
  } catch(e) { error.hidden = false; error.textContent = e.message; summary.textContent = 'Подбор не выполнен. Проверьте условия и повторите.'; document.querySelector('#count').textContent = 'Нет результата'; }
  finally { inFlight = false; submit.disabled = false; submit.textContent = 'Подобрать подрядчиков ↗'; }
});

document.querySelectorAll('[data-example]').forEach(button=>button.addEventListener('click',()=>{
  if(!ready || inFlight) return;
  const mode = button.dataset.example;
  form.elements.city.value='Алматы'; form.elements.date.value='2026-10-15'; form.elements.language.value=''; form.elements.hours.value='';
  form.elements.category.value=mode==='florist'?'Флорист':'Ведущий';
  form.elements.format.value=mode==='florist'?'свадьба':'корпоратив';
  form.elements.budget.value=mode==='empty'?'1000':mode==='florist'?'500000':'1000000';
  form.elements.wish.value=mode==='florist'?'Авторское оформление и современная эстетика':mode==='empty'?'Спокойный ведущий для делового вечера':'Спокойный ведущий без шаблонных конкурсов, с импровизацией и современным подходом';
  form.requestSubmit();
}));
const catalogReady = init();

const bundleForm = document.querySelector('#bundle-search');
const bundleResults = document.querySelector('#bundle-results');
const bundleSummary = document.querySelector('#bundle-summary');
const bundleCards = document.querySelector('#bundle-cards');
const bundleAlternatives = document.querySelector('#bundle-alternatives');
const bundleDurationBlock = document.querySelector('#bundle-durations');
const bundleDurationFields = document.querySelector('#bundle-duration-fields');

function renderBundleDurations() {
  const selected = [...bundleForm.querySelectorAll('input[name="category"]:checked')].map(input => input.value);
  const previous = Object.fromEntries([...bundleDurationFields.querySelectorAll('input[data-duration-category]')].map(input => [input.dataset.durationCategory, input.value]));
  bundleDurationFields.replaceChildren(...selected.map(category => {
    const label = el('label','duration-choice');
    label.append(el('span','',category));
    const input = document.createElement('input'); input.type = 'number'; input.min = '0'; input.step = '1'; input.value = previous[category] || '8'; input.dataset.durationCategory = category; input.setAttribute('aria-label', `Часов: ${category}`);
    label.append(input, document.createTextNode(' часов')); return label;
  }));
  bundleDurationBlock.hidden = selected.length === 0;
}

document.querySelector('#bundle-categories').addEventListener('change', renderBundleDurations);
renderBundleDurations();

document.querySelectorAll('.mode').forEach(button => button.addEventListener('click', () => {
  const bundle = button.dataset.mode === 'bundle';
  document.querySelectorAll('.mode').forEach(item => item.classList.toggle('active', item === button));
  form.hidden = bundle; bundleForm.hidden = !bundle; bundleResults.hidden = !bundle;
  document.querySelector('#cards').hidden = bundle; document.querySelector('.examples').hidden = bundle;
  if (bundle) { reasons.hidden = true; alternatives.hidden = true; }
  if (bundle) { document.querySelector('#result-title').textContent = 'Ваш комплект'; document.querySelector('#summary').textContent = 'Выберите категории и общий бюджет — соберём комплект из каталога.'; }
  else { document.querySelector('#result-title').textContent = 'Ваш короткий список'; }
}));

function renderBundleItem(item) {
  const article = el('article','card');
  const top = el('div','card-top'); top.append(el('div','avatar', item.contractor.name.split(' ').slice(0,2).map(w=>w[0]).join('')));
  const identity = el('div','identity'); identity.append(el('h3','',item.contractor.name),el('div','meta',item.category+' · '+item.contractor.city));
  top.append(identity,el('div','price',`от ${money(item.price)} ₸`)); article.append(top);
  article.append(el('div','explanation',item.explanation)); return article;
}

bundleForm.addEventListener('submit', async event => {
  event.preventDefault();
  const categories = [...bundleForm.querySelectorAll('input[name="category"]:checked')].map(input => input.value);
  if (!categories.length) { bundleSummary.textContent = 'Выберите хотя бы одну категорию.'; return; }
  const query = Object.fromEntries(new FormData(bundleForm)); delete query.category; delete query.hours;
  const params = new URLSearchParams(query); categories.forEach(category => params.append('category', category));
  bundleForm.querySelectorAll('input[data-duration-category]').forEach(input => params.append('duration', `${input.dataset.durationCategory}|${input.value}`));
  document.querySelector('#count').textContent = 'Поиск';
  summary.textContent = 'Проверяем условия и календарь…';
  bundleSummary.textContent = 'Собираем комплект из реальных профилей…'; bundleCards.replaceChildren(); bundleAlternatives.replaceChildren();
  try {
    const response = await fetch('/api/bundle?'+params); const result = await response.json();
    if (!response.ok) throw new Error(result.error || 'Не удалось собрать мероприятие.');
    document.querySelector('#count').textContent = `${result.items.length} из ${categories.length}`;
    summary.textContent = `${query.city} · ${query.date.split('-').reverse().join('.')} · ${query.format}.`;
    bundleSummary.textContent = result.message + ` Итог: ${money(result.total)} ₸.`;
    bundleCards.replaceChildren(...result.items.map(renderBundleItem));
    if (result.alternatives?.length) {
      const heading = el('h3','', 'Почти идеальные варианты'); bundleAlternatives.append(heading);
      result.alternatives.forEach(alternative => { const box = el('article','alternative'); box.append(el('h4','',alternative.kind === 'other_date' ? 'Если готовы изменить дату' : 'Если готовы немного увеличить бюджет'),el('p','',alternative.message),el('p','',`Итог комплекта: ${money(alternative.total)} ₸`)); bundleAlternatives.append(box); });
    }
  } catch (e) { bundleSummary.textContent = e.message; summary.textContent = 'Подбор не выполнен. Проверьте условия и повторите.'; document.querySelector('#count').textContent = 'Нет результата'; }
});

const aiQuery = document.querySelector('#ai-query');
const aiTextSubmit = document.querySelector('#ai-text-submit');
const aiVoiceSubmit = document.querySelector('#ai-voice-submit');
const aiStatus = document.querySelector('#ai-status');
const aiUnderstood = document.querySelector('#ai-understood');
let recorder;
let recording = [];

function showAIStatus(message, isError = false) { aiStatus.hidden = false; aiStatus.textContent = message; aiStatus.className = isError ? 'ai-error' : ''; }
function renderUnderstood(parsed, transcript = '') {
  const fields = [['Режим', parsed.mode === 'bundle' ? 'Собрать мероприятие' : 'Найти подрядчика'], ['Город', parsed.city], ['Дата', parsed.date], ['Формат', parsed.eventFormat], ['Категория', parsed.category || (parsed.categories || []).join(', ')], ['Бюджет', parsed.budgetKzt ? money(parsed.budgetKzt)+' ₸' : parsed.totalBudgetKzt ? money(parsed.totalBudgetKzt)+' ₸' : ''], ['Пожелания', parsed.preferences]];
  aiUnderstood.hidden = false; aiUnderstood.replaceChildren(el('strong','',transcript ? `Распознано: «${transcript}»` : 'Мы поняли ваш запрос'), ...fields.filter(([,value]) => value).map(([label,value]) => el('span','',`${label}: ${value}`)));
}
async function applyParsed(parsed, transcript = '') {
  await catalogReady;
  if (!ready) throw new Error('Не удалось загрузить каталог. Обновите страницу.');
  renderUnderstood(parsed, transcript);
  const bundle = parsed.mode === 'bundle';
  document.querySelector(`.mode[data-mode="${bundle ? 'bundle' : 'single'}"]`).click();
  const targetForm = bundle ? bundleForm : form;
  targetForm.elements.city.value = parsed.city || '';
  targetForm.elements.date.value = parsed.date || '';
  targetForm.elements.format.value = parsed.eventFormat || '';
  targetForm.elements.budget.value = (bundle ? parsed.totalBudgetKzt : parsed.budgetKzt) || '';
  targetForm.elements.language.value = parsed.language || '';
  if (!bundle) {
    form.elements.category.value = parsed.category || '';
    form.elements.hours.value = parsed.durationHours || '';
    form.elements.wish.value = parsed.preferences || '';
  } else {
    const wanted = parsed.categories || []; bundleForm.querySelectorAll('input[name="category"]').forEach(input => { input.checked = wanted.includes(input.value); }); renderBundleDurations();
    bundleForm.querySelectorAll('input[data-duration-category]').forEach(input => {
      input.value = parsed.categoryDurations?.[input.dataset.durationCategory] ?? parsed.durationHours ?? 0;
    });
  }
  const required = [['city', 'город'], ['date', 'дату'], ['format', 'формат мероприятия'], ['budget', bundle ? 'общий бюджет' : 'бюджет']];
  if (!bundle) required.push(['category', 'категорию подрядчика']);
  const missing = required.filter(([name]) => !targetForm.elements[name].value).map(([, label]) => label);
  if (bundle && !bundleForm.querySelector('input[name="category"]:checked')) missing.push('категории подрядчиков');
  if (missing.length) {
    cards.replaceChildren(); bundleCards.replaceChildren(); bundleAlternatives.replaceChildren();
    reasons.hidden = true; alternatives.hidden = true;
    const message = `Укажите ${missing.join(', ')} в форме и нажмите «${bundle ? 'Собрать мероприятие' : 'Подобрать подрядчиков'}». Остальные параметры уже заполнены.`;
    summary.textContent = message; bundleSummary.textContent = message;
    document.querySelector('#count').textContent = 'Нужны уточнения';
    showAIStatus(message);
    return;
  }
  if (!targetForm.reportValidity()) {
    showAIStatus('Проверьте выделенное поле в форме: дата должна быть в пределах каталога, бюджет — положительным целым числом.', true);
    return;
  }
  targetForm.requestSubmit();
  showAIStatus('Запрос распознан. Подбираем подрядчиков по вашим условиям.');
}
async function parseText(text, transcript = '') {
  showAIStatus('Обрабатываем запрос…'); aiTextSubmit.disabled = true; aiVoiceSubmit.disabled = true;
  try { const response = await fetch('/api/ai/parse',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({text})}); const result = await response.json(); if (!response.ok) throw new Error(result.error || 'Не удалось обработать запрос.'); await applyParsed(result.parsed, transcript); } catch (error) { showAIStatus(error.message, true); } finally { aiTextSubmit.disabled = false; aiVoiceSubmit.disabled = false; }
}
aiTextSubmit.addEventListener('click', () => { if (aiQuery.value.trim()) parseText(aiQuery.value.trim()); else showAIStatus('Введите текстовый запрос.', true); });
aiVoiceSubmit.addEventListener('click', async () => {
  if (recorder && recorder.state === 'recording') { recorder.stop(); aiVoiceSubmit.textContent = '🎙 Записать голос'; return; }
  if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) { showAIStatus('Голосовой ввод не поддерживается. Используйте текстовый поиск.', true); return; }
  try { const stream = await navigator.mediaDevices.getUserMedia({audio:true}); recording = []; recorder = new MediaRecorder(stream); recorder.ondataavailable = event => { if (event.data.size) recording.push(event.data); }; recorder.onstop = async () => { stream.getTracks().forEach(track => track.stop()); const blob = new Blob(recording,{type:recorder.mimeType || 'audio/webm'}); const formData = new FormData(); formData.append('audio',blob,'voice.webm'); showAIStatus('Распознаём голос…'); try { const response = await fetch('/api/ai/voice',{method:'POST',body:formData}); const result = await response.json(); if (!response.ok) throw new Error(result.error || 'Не удалось распознать голосовой запрос.'); await applyParsed(result.parsed,result.transcript); } catch (error) { showAIStatus(error.message, true); } }; recorder.start(); aiVoiceSubmit.textContent = '⏹ Остановить запись'; showAIStatus('Идёт запись…'); } catch { showAIStatus('Доступ к микрофону запрещён. Используйте текстовый поиск.', true); }
});
