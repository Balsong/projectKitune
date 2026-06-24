/* ============================================================
   КИЦУНЭ — Чайный магазин (отдельный контекст: своя корзина и API)
   ============================================================ */
(function () {
  "use strict";
  const CART_KEY = "kitsune_shop_cart_id";

  function cartId() {
    let i = localStorage.getItem(CART_KEY);
    if (!i) {
      i = crypto.randomUUID ? crypto.randomUUID() : String(Date.now()) + Math.random().toString(16).slice(2);
      localStorage.setItem(CART_KEY, i);
    }
    return i;
  }

  // Заголовки: своя корзина магазина + (если есть) токен авторизации.
  const auth = () => (window.KitsuneAuth ? window.KitsuneAuth.headers() : {});
  const H = () => Object.assign({ "Content-Type": "application/json", "X-Shop-Cart-Id": cartId() }, auth());

  const ShopAPI = {
    menu: (category) =>
      fetch("/api/v1/shop/menu" + (category ? "?category=" + encodeURIComponent(category) : "")).then((r) => r.json()),
    cart: () => fetch("/api/v1/shop/cart", { headers: H() }).then((r) => r.json()),
    add: (id, q) =>
      fetch("/api/v1/shop/cart/items", { method: "POST", headers: H(), body: JSON.stringify({ product_id: id, quantity: q }) }).then((r) => r.json()),
    del: (id, q) =>
      fetch("/api/v1/shop/cart/items", { method: "DELETE", headers: H(), body: JSON.stringify({ product_id: id, quantity: q }) }).then((r) => r.json()),
    clear: () => fetch("/api/v1/shop/cart/clear", { method: "POST", headers: H() }).then((r) => r.json()),
    checkout: (body) => fetch("/api/v1/shop/checkout", { method: "POST", headers: H(), body: JSON.stringify(body) }),
    myOrders: () => fetch("/api/v1/shop/orders", { headers: auth() }).then((r) => (r.ok ? r.json() : [])),
  };
  window.KitsuneShop = ShopAPI;

  // Бейдж количества в корзине магазина (элементы с [data-shop-cart-count]).
  window.kitsuneShopBadge = async function () {
    try {
      const c = await ShopAPI.cart();
      const n = (c.items || []).reduce((s, i) => s + i.quantity, 0);
      document.querySelectorAll("[data-shop-cart-count]").forEach((el) => {
        el.textContent = n;
        if (n) el.removeAttribute("data-empty");
        else el.setAttribute("data-empty", "1");
      });
    } catch (e) {}
  };

  document.addEventListener("DOMContentLoaded", window.kitsuneShopBadge);
})();
