<script setup lang="ts">
// Дизайн «Прил-Генератор» — самодостаточная страница со своим Tailwind (CDN),
// шрифтами и стилями. Держим её в iframe (public/generator/index.html), чтобы
// её оформление не конфликтовало с дизайн-токенами основного приложения.
</script>

<template>
  <iframe
    src="/generator/index.html"
    title="Генератор гипотез"
    class="fixed inset-0 h-screen w-screen border-0"
  />
</template>
