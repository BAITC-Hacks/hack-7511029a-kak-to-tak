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
    form.elements.city.value = 'Алматы'; form.elements.category.value = 'Ведущий'; form.elements.format.value = 'корпоратив';
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
init();
