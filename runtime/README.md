# Serveur d'inférence local

Pour compiler `llama-server` depuis les sources officielles figées dans le projet :

```sh
./scripts/build-llama-server.sh
# Équivalent via le Makefile : make llama-server
```

Le script s'exécute depuis n'importe quel répertoire. Il exige Git, Python 3 avec `venv`/`pip`, Make et un compilateur C/C++. Sous macOS, Xcode ou les Command Line Tools doivent être installés. Le premier lancement télécharge les sources et installe CMake **3.31.6** dans `.tools/llama-build/`. Les compilations suivantes réutilisent ces fichiers et le cache de compilation. Aucun outil n'est installé globalement.

Organisation :

| Emplacement | Contenu |
| --- | --- |
| `third_party/llama.cpp/` | Clone officiel, révision vérifiée |
| `.tools/llama-build/` | Python et CMake locaux |
| `build/llama.cpp/` | Cache CMake, objets et bibliothèques statiques |
| `runtime/llama-server` | Binaire natif compilé |
| `runtime/llama-server.build.json` | Révision, architecture, options principales et SHA-256 du binaire |
| `runtime/llama.cpp.LICENSE` | Licence de llama.cpp |

La compilation utilise quatre tâches en parallèle. Pour ajuster ce nombre :

```sh
LLAMA_BUILD_JOBS=8 ./scripts/build-llama-server.sh
```

Le binaire est compilé en mode Release. Sur macOS, Metal et Accelerate sont activés ; les shaders Metal sont embarqués et les bibliothèques llama/ggml sont liées statiquement. Les frameworks du système restent nécessaires. Sous Linux, ce script sélectionne le backend CPU. Il compile uniquement la cible `llama-server`, sans interface Web embarquée ni prise en charge HTTPS ; l'API HTTP locale est adaptée au jeu. Les modèles ne sont pas téléchargés par ce script.

Vérification et utilisation depuis la racine du workspace :

```sh
./runtime/llama-server --version
./runtime/llama-server --list-devices
go run ./cmd/aventure -provider llamacpp -gguf ./models/lfm3-1.7b.gguf
```

Remplacer le chemin GGUF par un fichier de modèle existant. Le jeu cherche d'abord `runtime/llama-server` à côté de son exécutable ou de son dossier `bin/`, puis dans le répertoire de travail, puis dans le `PATH`. `-llama-bin` permet toujours d'imposer un chemin. Le fournisseur par défaut reste Ollama ; `-provider llamacpp` sélectionne ce serveur.

Les sources, outils, objets, manifeste de compilation et binaire sont ignorés par Git. Le script, les explications et la licence restent versionnables. Le binaire généré cible la plateforme qui effectue la compilation ; il faut le reconstruire pour une autre plateforme.

`make dist VERSION=0.1.0` compile le jeu et ce runtime, puis crée une archive native dans `dist/`, avec les notices et licences, les métadonnées et un guide destiné au joueur. Les modèles GGUF restent séparés. `make dist WITH_LLAMA=0` produit une archive du jeu seul et permet la compilation Go vers une autre plateforme.
