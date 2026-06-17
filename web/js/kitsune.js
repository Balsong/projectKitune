/* ============================================================
   КИЦУНЭ — core (cart · ui · reveal · tweaks)
   ============================================================ */
(function(){
  "use strict";
  const CART_KEY = "kitsune_cart_v1";
  const FAV_KEY  = "kitsune_fav_v1";

  /* ---------- storage ---------- */
  const read = (k, fb) => { try { return JSON.parse(localStorage.getItem(k)) ?? fb; } catch(e){ return fb; } };
  const write = (k, v) => { try { localStorage.setItem(k, JSON.stringify(v)); } catch(e){} };

  /* ---------- backend API (тот же origin, /api проксируется nginx → gateway) ---------- */
  const CARTID_KEY = "kitsune_cart_id";
  function cartId(){
    let i = localStorage.getItem(CARTID_KEY);
    if(!i){ i = (crypto.randomUUID ? crypto.randomUUID() : String(Date.now())+Math.random().toString(16).slice(2)); localStorage.setItem(CARTID_KEY, i); }
    return i;
  }
  const H = () => ({ "Content-Type":"application/json", "X-Cart-Id": cartId() });
  const API = {
    menu:  () => fetch("/api/v1/menu").then(r=>r.json()),
    cart:  () => fetch("/api/v1/cart", {headers:H()}).then(r=>r.json()),
    add:   (id,q) => fetch("/api/v1/cart/items", {method:"POST",   headers:H(), body:JSON.stringify({product_id:id, quantity:q})}).then(r=>r.json()),
    del:   (id,q) => fetch("/api/v1/cart/items", {method:"DELETE", headers:H(), body:JSON.stringify({product_id:id, quantity:q})}).then(r=>r.json()),
    clear: () => fetch("/api/v1/cart/clear", {method:"POST", headers:H()}).then(r=>r.json()),
    order: (b) => fetch("/api/v1/orders", {method:"POST", headers:H(), body:JSON.stringify(b)}),
    getOrder: (id) => fetch("/api/v1/orders/"+id).then(r=>r.json()),
    getDelivery: (id) => fetch("/api/v1/orders/"+id+"/delivery").then(r=> r.ok ? r.json() : null)
  };
  window.KitsuneAPI = API;
  window.kitsuneResetCart = () => localStorage.setItem(CARTID_KEY, crypto.randomUUID ? crypto.randomUUID() : String(Date.now()));

  /* ---------- аутентификация (сессия в Redis на бэке, токен в localStorage) ---------- */
  const TOKEN_KEY = "kitsune_token";
  const Auth = {
    token(){ return localStorage.getItem(TOKEN_KEY) || ""; },
    setToken(t){ if(t) localStorage.setItem(TOKEN_KEY, t); else localStorage.removeItem(TOKEN_KEY); },
    isAuthed(){ return !!this.token(); },
    headers(){ const t = this.token(); return t ? { "Authorization": "Bearer " + t } : {}; },
    async register(body){
      const r = await fetch("/api/v1/auth/register", { method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify(body) });
      const d = await r.json().catch(()=>({}));
      if(!r.ok) throw new Error(d.error || ("HTTP "+r.status));
      this.setToken(d.session_token); return d.user;
    },
    async login(email, password){
      const r = await fetch("/api/v1/auth/login", { method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({email, password}) });
      const d = await r.json().catch(()=>({}));
      if(!r.ok) throw new Error(d.error || ("HTTP "+r.status));
      this.setToken(d.session_token); return d.user;
    },
    async me(){
      if(!this.token()) return null;
      try{
        const r = await fetch("/api/v1/auth/me", { headers: this.headers() });
        if(!r.ok){ this.setToken(""); return null; }
        return await r.json();
      }catch(e){ return null; }
    },
    async logout(){
      try{ await fetch("/api/v1/auth/logout", { method:"POST", headers: this.headers() }); }catch(e){}
      this.setToken("");
    }
  };
  window.KitsuneAuth = Auth;

  /* sku (id из дизайна) <-> backend UUID, из /api/v1/menu */
  let SKU2ID = {}, ID2SKU = {}, MENU_READY = null;
  function loadMenu(){
    if(MENU_READY) return MENU_READY;
    MENU_READY = API.menu().then(list=>{
      (list||[]).forEach(p=>{ if(p.sku){ SKU2ID[p.sku]=p.id; ID2SKU[p.id]=p.sku; } });
    }).catch(()=>{});
    return MENU_READY;
  }
  window.KitsuneMenuReady = loadMenu;

  /* ---------- cart (backend-backed, с локальным кэшем для синхронных чтений) ---------- */
  const Cart = {
    _state: [],  // [{id:sku, qty, name, price(₽), lineCents}]
    items(){ return this._state.map(i=>({id:i.id, qty:i.qty})); },
    count(){ return this._state.reduce((s,i)=>s+i.qty,0); },
    total(){ return this._state.reduce((s,i)=>s+i.lineCents,0)/100; },
    async refresh(){
      await loadMenu();
      try{
        const c = await API.cart();
        this._state = (c.items||[]).map(it=>({
          id: ID2SKU[it.product_id] || it.product_id,
          qty: it.quantity, name: it.name,
          price: (it.unit_price_cents||0)/100, lineCents: it.subtotal_cents||0
        }));
      }catch(e){ this._state = []; }
      this.sync();
      return this._state;
    },
    async add(sku, qty=1){ await loadMenu(); const id=SKU2ID[sku]; if(!id) return; await API.add(id, qty); await this.refresh(); },
    async setQty(sku, qty){
      await loadMenu(); const id=SKU2ID[sku]; if(!id) return;
      const cur = (this._state.find(i=>i.id===sku)||{}).qty || 0;
      if(qty<=0) await API.del(id, 0);
      else if(qty>cur) await API.add(id, qty-cur);
      else if(qty<cur) await API.del(id, cur-qty);
      await this.refresh();
    },
    async remove(sku){ await loadMenu(); const id=SKU2ID[sku]; if(!id) return; await API.del(id, 0); await this.refresh(); },
    async clear(){ try{ await API.clear(); }catch(e){} await this.refresh(); },
    sync(){
      document.querySelectorAll("[data-cart-count]").forEach(el=>{
        const c = this.count(); el.textContent = c; el.dataset.empty = c>0?"0":"1";
      });
      document.dispatchEvent(new CustomEvent("cart:change"));
    }
  };
  window.KitsuneCart = Cart;

  /* ---------- favourites ---------- */
  const Fav = {
    items(){ return read(FAV_KEY, []); },
    has(id){ return this.items().includes(id); },
    toggle(id){ let f=this.items(); f.includes(id)? f=f.filter(x=>x!==id): f.push(id); write(FAV_KEY,f); return f.includes(id); }
  };
  window.KitsuneFav = Fav;

  /* ---------- toast ---------- */
  let toastWrap;
  function toast(msg){
    if(!toastWrap){ toastWrap=document.createElement("div"); toastWrap.className="toast-wrap"; document.body.appendChild(toastWrap); }
    const t=document.createElement("div"); t.className="toast";
    t.innerHTML='<span class="dot"></span>'+msg;
    toastWrap.appendChild(t);
    requestAnimationFrame(()=>t.classList.add("show"));
    setTimeout(()=>{ t.classList.remove("show"); setTimeout(()=>t.remove(),350); }, 2200);
  }
  window.kitsuneToast = toast;

  /* ---------- product card markup ---------- */
  function cardHTML(p){
    const fav = Fav.has(p.id);
    const badge = p.badge ? `<span class="badge ${p.badge==='новинка'?'badge--sage':p.badge==='остро'?'':p.badge==='сет'?'badge--ochre':p.badge==='шеф'?'badge--ink':''}">${p.badge}</span>` : "";
    return `<article class="card reveal" data-id="${p.id}" data-cat="${p.cat}" data-name="${p.name.toLowerCase()}">
      <div class="card__media">
        <div class="ph" data-label="${p.ph}"></div>
        ${badge}
        <button class="card__fav ${fav?'is-fav':''}" data-fav="${p.id}" aria-label="В избранное">${heart(fav)}</button>
      </div>
      <div class="card__body">
        <div class="card__meta">${p.zh} · ${p.meta}</div>
        <h3 class="card__title">${p.name}</h3>
        <p class="card__desc">${p.desc}</p>
        <div class="card__foot">
          <div class="price">${window.rub(p.price)} <small>${p.unit}</small></div>
          <button class="add-btn" data-add="${p.id}">${plus()} В корзину</button>
        </div>
      </div>
    </article>`;
  }
  window.kitsuneCardHTML = cardHTML;

  const heart = on => `<svg width="17" height="17" viewBox="0 0 24 24" fill="${on?'currentColor':'none'}" stroke="currentColor" stroke-width="2"><path d="M12 21s-7.5-4.6-10-9C.5 9 2 5 5.5 5 8 5 9.5 6.5 12 9c2.5-2.5 4-4 6.5-4C22 5 23.5 9 22 12c-2.5 4.4-10 9-10 9z"/></svg>`;
  const plus  = () => `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>`;

  /* render helper */
  window.kitsuneRender = function(sel, list){
    const el = typeof sel==="string"? document.querySelector(sel) : sel;
    if(!el) return;
    el.innerHTML = list.map(cardHTML).join("");
    observeReveal(el);
  };

  /* live search + category filter across one or more grids */
  window.kitsuneMenuFilter = function(opts){
    const input = document.querySelector(opts.input);
    const chips = opts.chips ? Array.from(document.querySelectorAll(opts.chips)) : [];
    const cards = () => Array.from(document.querySelectorAll(opts.cards));
    const empty = opts.empty ? document.querySelector(opts.empty) : null;
    let q="", cat="all";
    function apply(){
      let shown=0;
      cards().forEach(c=>{
        const okCat = cat==="all" || c.dataset.cat===cat;
        const okQ = !q || (c.dataset.name||"").includes(q);
        const sec = okCat && okQ;
        c.style.display = sec ? "" : "none"; if(sec) shown++;
      });
      // hide whole sections that have no visible cards
      if(opts.sections){
        document.querySelectorAll(opts.sections).forEach(s=>{
          const any = s.querySelectorAll(opts.cards+':not([style*="display: none"])').length;
          s.style.display = any ? "" : "none";
        });
      }
      if(empty) empty.style.display = shown ? "none" : "";
      observeReveal();
    }
    if(input) input.addEventListener("input", e=>{ q=e.target.value.trim().toLowerCase(); apply(); });
    chips.forEach(ch=>ch.addEventListener("click", ()=>{
      chips.forEach(x=>x.classList.remove("is-active"));
      ch.classList.add("is-active"); cat=ch.dataset.cat||"all"; apply();
    }));
    // prefill from ?q=
    const url = new URLSearchParams(location.search).get("q");
    if(url && input){ input.value=url; q=url.trim().toLowerCase(); apply(); }
  };

  /* ---------- delegated clicks ---------- */
  document.addEventListener("click", e=>{
    const add = e.target.closest("[data-add]");
    if(add){
      const id = add.dataset.add; const p = window.KITSUNE_FIND(id);
      Cart.add(id,1);
      add.classList.add("is-added");
      const orig = add.innerHTML; add.innerHTML = check()+" Добавлено";
      setTimeout(()=>{ add.classList.remove("is-added"); add.innerHTML = orig; }, 1300);
      toast(`«${p?p.name:'Товар'}» — в корзине`);
    }
    const fav = e.target.closest("[data-fav]");
    if(fav){
      const on = Fav.toggle(fav.dataset.fav);
      fav.classList.toggle("is-fav", on);
      fav.innerHTML = heart(on);
    }
  });
  const check = () => `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>`;

  /* ---------- reveal on scroll (synchronous + setTimeout failsafe for frozen iframes) ---------- */
  let revealT = 0;
  function revealCheck(){
    const vh = window.innerHeight || document.documentElement.clientHeight;
    document.querySelectorAll(".reveal:not(.in)").forEach(el=>{
      const r = el.getBoundingClientRect();
      if(r.top < vh * 0.94 && r.bottom > -40){
        el.classList.add("in");
        // failsafe: guarantee final visible state even if the CSS transition
        // never advances (background/throttled iframe). In a live foreground
        // browser the .7s transition has already completed by now → no-op.
        setTimeout(()=>el.classList.add("reveal-done"), 780);
      }
    });
  }
  function queueReveal(){
    revealCheck();
    clearTimeout(revealT); revealT = setTimeout(revealCheck, 120);
  }
  function observeReveal(){ revealCheck(); }
  window.kitsuneObserveReveal = observeReveal;

  /* ---------- header scroll + mobile drawer ---------- */
  function initChrome(){
    const header = document.querySelector(".site-header");
    if(header){ const onScroll=()=>header.classList.toggle("is-scrolled", window.scrollY>8); onScroll(); window.addEventListener("scroll", onScroll, {passive:true}); }
    const burger = document.querySelector("[data-burger]");
    const drawer = document.querySelector("[data-drawer]");
    if(burger && drawer){
      const open=()=>drawer.classList.add("is-open");
      const close=()=>drawer.classList.remove("is-open");
      burger.addEventListener("click", open);
      drawer.addEventListener("click", e=>{ if(e.target.matches("[data-drawer-close],.drawer__scrim")) close(); });
    }
  }

  /* ---------- tweaks (vanilla panel + host protocol, site-wide via localStorage) ---------- */
  const Tweaks = (function(){
    const LS = "kitsune_tweaks";
    const PALETTES = [
      ["#D23A53","#B82F46","#9A2539","#F8DEE3"],
      ["#26201A","#171210","#000000","#E9E3DA"],
      ["#8E2F3E","#742533","#5C1D29","#EFD8DC"]
    ];
    const PNAMES = ["Малиновый","Графит","Винный"];
    const FONTS = [
      ['"Playfair Display", Georgia, serif', "Playfair"],
      ['"Cormorant Garamond", Georgia, serif', "Cormorant"],
      ['"Lora", Georgia, serif', "Lora"]
    ];
    const DEF = { accent:0, radius:16, font:0 };
    const get = () => Object.assign({}, DEF, read(LS, {}));
    function apply(){
      const s = get(), r = document.documentElement.style;
      const a = PALETTES[s.accent] || PALETTES[0];
      r.setProperty("--accent",a[0]); r.setProperty("--accent-600",a[1]);
      r.setProperty("--accent-700",a[2]); r.setProperty("--accent-tint",a[3]);
      r.setProperty("--radius", s.radius+"px"); r.setProperty("--radius-sm", Math.max(4,s.radius-6)+"px");
      r.setProperty("--font-head", (FONTS[s.font]||FONTS[0])[0]);
    }
    function set(k,v){ const s=get(); s[k]=v; write(LS,s); apply(); render(); }
    let panel, open=false;
    function build(){
      panel = document.createElement("div"); panel.className="twkx"; panel.setAttribute("data-omelette-chrome","");
      panel.innerHTML = `
        <div class="twkx__hd"><b>Tweaks · Кицунэ</b><button class="twkx__x" aria-label="Закрыть">✕</button></div>
        <div class="twkx__body">
          <div class="twkx__sect">Палитра акцента</div>
          <div class="twkx__sw" data-sw></div>
          <div class="twkx__sect">Скругление карточек</div>
          <div class="twkx__row"><input type="range" min="2" max="24" step="1" data-radius><span class="twkx__val" data-radius-val></span></div>
          <div class="twkx__sect">Шрифт заголовков</div>
          <select class="twkx__sel" data-font>${FONTS.map((f,i)=>`<option value="${i}">${f[1]}</option>`).join("")}</select>
        </div>`;
      const css = document.createElement("style"); css.textContent = TWKX_CSS; document.head.appendChild(css);
      document.body.appendChild(panel);
      panel.querySelector(".twkx__x").addEventListener("click", dismiss);
      panel.querySelector("[data-radius]").addEventListener("input", e=>set("radius", +e.target.value));
      panel.querySelector("[data-font]").addEventListener("change", e=>set("font", +e.target.value));
      panel.querySelector("[data-sw]").addEventListener("click", e=>{ const b=e.target.closest("[data-i]"); if(b) set("accent", +b.dataset.i); });
    }
    function render(){
      if(!panel) return; const s=get();
      panel.querySelector("[data-sw]").innerHTML = PALETTES.map((p,i)=>
        `<button data-i="${i}" class="twkx__chip ${i===s.accent?'is-on':''}" title="${PNAMES[i]}" style="--c:${p[0]}"><i style="background:${p[0]}"></i>${PNAMES[i]}</button>`).join("");
      panel.querySelector("[data-radius]").value = s.radius;
      panel.querySelector("[data-radius-val]").textContent = s.radius+"px";
      panel.querySelector("[data-font]").value = s.font;
    }
    function show(){ if(!panel) build(); render(); panel.classList.add("is-open"); open=true; }
    function hide(){ if(panel) panel.classList.remove("is-open"); open=false; }
    function dismiss(){ hide(); try{ window.parent.postMessage({type:"__edit_mode_dismissed"},"*"); }catch(e){} }
    function init(){
      apply();
      window.addEventListener("message", e=>{
        const t=e&&e.data&&e.data.type;
        if(t==="__activate_edit_mode") show();
        else if(t==="__deactivate_edit_mode") hide();
      });
      window.addEventListener("storage", e=>{ if(e.key===LS){ apply(); render(); } });
      try{ window.parent.postMessage({type:"__edit_mode_available"},"*"); }catch(e){}
    }
    return { init, apply };
  })();
  const TWKX_CSS = `
    .twkx{position:fixed;right:16px;bottom:16px;z-index:2147483646;width:264px;
      background:rgba(252,249,242,.86);-webkit-backdrop-filter:blur(20px) saturate(1.5);backdrop-filter:blur(20px) saturate(1.5);
      border:1px solid rgba(43,38,34,.12);border-radius:14px;box-shadow:0 16px 44px rgba(43,38,34,.26);
      font-family:var(--font-body);color:var(--ink);overflow:hidden;opacity:0;transform:translateY(10px);pointer-events:none;transition:.2s}
    .twkx.is-open{opacity:1;transform:none;pointer-events:auto}
    .twkx__hd{display:flex;align-items:center;justify-content:space-between;padding:12px 10px 12px 15px;border-bottom:1px solid rgba(43,38,34,.08)}
    .twkx__hd b{font-size:13px;font-weight:600}
    .twkx__x{width:24px;height:24px;border-radius:6px;color:var(--ink-3);font-size:13px}
    .twkx__x:hover{background:rgba(43,38,34,.07);color:var(--ink)}
    .twkx__body{padding:6px 15px 16px}
    .twkx__sect{font-family:var(--font-mono);font-size:10px;letter-spacing:.12em;text-transform:uppercase;color:var(--ink-3);margin:14px 0 8px}
    .twkx__sw{display:flex;flex-direction:column;gap:6px}
    .twkx__chip{display:flex;align-items:center;gap:9px;padding:8px 10px;border:1.5px solid var(--line);border-radius:9px;background:var(--paper-card);font-size:13px;font-weight:500;color:var(--ink-2);transition:.15s}
    .twkx__chip i{width:16px;height:16px;border-radius:5px;box-shadow:inset 0 0 0 1px rgba(0,0,0,.08)}
    .twkx__chip:hover{border-color:var(--c)}
    .twkx__chip.is-on{border-color:var(--c);color:var(--ink);box-shadow:0 0 0 3px color-mix(in srgb,var(--c) 16%,transparent)}
    .twkx__row{display:flex;align-items:center;gap:10px}
    .twkx__row input[type=range]{flex:1;accent-color:var(--accent)}
    .twkx__val{font-family:var(--font-mono);font-size:12px;color:var(--ink-3);min-width:34px;text-align:right}
    .twkx__sel{width:100%;padding:8px 10px;border:1.5px solid var(--line);border-radius:9px;background:var(--paper-card);font-size:13px;font-family:var(--font-body)}
  `;
  window.kitsuneApplyTweaks = () => Tweaks.apply();

  /* ---------- boot ---------- */
  function boot(){
    Tweaks.init();
    initChrome();
    observeReveal();
    window.addEventListener("scroll", queueReveal, {passive:true});
    window.addEventListener("resize", queueReveal);
    window.addEventListener("load", queueReveal);
    Cart.refresh();
  }
  if(document.readyState==="loading") document.addEventListener("DOMContentLoaded", boot);
  else boot();
})();
