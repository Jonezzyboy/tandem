import { mount } from 'svelte'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import '@fontsource-variable/bricolage-grotesque'
import './themes.css'
import './app.css'
import App from './App.svelte'

mount(App, { target: document.getElementById('app')! })
