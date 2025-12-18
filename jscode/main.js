global.cursor_config = {
    currentScriptSrc: "$$currentScriptSrc$$",
    fp:{
        UNMASKED_VENDOR_WEBGL:"$$UNMASKED_VENDOR_WEBGL$$",
        UNMASKED_RENDERER_WEBGL:"$$UNMASKED_RENDERER_WEBGL$$",
        userAgent: "$$userAgent$$"
    }
}

$$env_jscode$$

let console_log = console.log;
console.log = function () {

}

dtavm = console;
delete __dirname;
delete __filename;

function proxy(obj, objname, type) {
    function getMethodHandler(WatchName, target_obj) {
        let methodhandler = {
            apply(target, thisArg, argArray) {
                if (this.target_obj) {
                    thisArg = this.target_obj
                }
                let result = Reflect.apply(target, thisArg, argArray)
                return result
            },
            construct(target, argArray, newTarget) {
                var result = Reflect.construct(target, argArray, newTarget)
                return result;
            }
        }
        methodhandler.target_obj = target_obj
        return methodhandler
    }

    function getObjhandler(WatchName) {
        let handler = {
            get(target, propKey, receiver) {
                let result = target[propKey]
                if (result instanceof Object) {
                    if (typeof result === "function") {
                        return new Proxy(result, getMethodHandler(WatchName, target))
                    }
                    return new Proxy(result, getObjhandler(`${WatchName}.${propKey}`))
                }
                return result;
            },
            set(target, propKey, value, receiver) {
                return Reflect.set(target, propKey, value, receiver);
            },
            has(target, propKey) {
                var result = Reflect.has(target, propKey);
                return result;
            },
            deleteProperty(target, propKey) {
                var result = Reflect.deleteProperty(target, propKey);
                return result;
            },
            defineProperty(target, propKey, attributes) {
                var result = Reflect.defineProperty(target, propKey, attributes);
                return result
            },
            getPrototypeOf(target) {
                var result = Reflect.getPrototypeOf(target)
                return result;
            },
            setPrototypeOf(target, proto) {
                return Reflect.setPrototypeOf(target, proto);
            },
            preventExtensions(target) {
                return Reflect.preventExtensions(target);
            },
            isExtensible(target) {
                var result = Reflect.isExtensible(target)
                return result;
            },
        }
        return handler;
    }

    if (type === "method") {
        return new Proxy(obj, getMethodHandler(objname, obj));
    }
    return new Proxy(obj, getObjhandler(objname));
}

global.document = window.document;

$$cursor_jscode$$

window.V_C[0]().then(value => console_log(JSON.stringify(value)));
